package anthropicapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/ports"
)

// TestClient_Fetch_KeychainExpired_NeverRefreshesNeverPersists is the core
// safety guarantee: when the expired token was read from the Keychain and
// Claude Code has not refreshed it, clyde must NOT rotate the shared refresh
// token (which could log the user out of the Claude Code CLI) and must NOT
// write the file copy. It declines with the graceful "plan offline" error.
func TestClient_Fetch_KeychainExpired_NeverRefreshesNeverPersists(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	_, hc := newServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("no network call expected for an expired Keychain token; got %s", r.URL.Path)
	})

	c := NewClientWithDeps(hc, fakeClock(now))
	var saved int32
	c.saveFile = func(Credentials) error { atomic.AddInt32(&saved, 1); return nil }
	// Keychain source, expired on every read (Claude Code hasn't refreshed).
	c.loadCreds = func() (Credentials, credSource, error) {
		return Credentials{
			AccessToken:  "kc-expired",
			RefreshToken: "kc-rt",
			ExpiresAt:    now.Add(-time.Hour),
		}, sourceKeychain, nil
	}

	_, err := c.Fetch(context.Background())
	if !errors.Is(err, ports.ErrPlanUsageUnavailable) {
		t.Errorf("Keychain expired: err = %v, want ports.ErrPlanUsageUnavailable", err)
	}
	if got := atomic.LoadInt32(&saved); got != 0 {
		t.Errorf("saveFile called %d times; a Keychain-sourced token must never be persisted", got)
	}
}

// TestClient_Fetch_KeychainExpired_PicksUpClaudeCodeRefresh proves the
// read-only recovery path: when the cached Keychain token is expired, clyde
// re-reads the Keychain and uses whatever fresh token Claude Code has placed
// there — no refresh, no persist.
func TestClient_Fetch_KeychainExpired_PicksUpClaudeCodeRefresh(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	_, hc := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth/token":
			t.Errorf("refresh endpoint must NOT be hit; Keychain is read-only for clyde")
		case "/api/oauth/usage":
			if got := r.Header.Get("Authorization"); got != "Bearer kc-fresh" {
				t.Errorf("Authorization = %q, want Bearer kc-fresh (Claude Code's refreshed token)", got)
			}
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":5},"seven_day":{"utilization":6}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	c := NewClientWithDeps(hc, fakeClock(now))
	var saved int32
	c.saveFile = func(Credentials) error { atomic.AddInt32(&saved, 1); return nil }
	var reads int32
	c.loadCreds = func() (Credentials, credSource, error) {
		if atomic.AddInt32(&reads, 1) == 1 {
			// Initial load: the token Claude Code left is already expired.
			return Credentials{AccessToken: "kc-expired", ExpiresAt: now.Add(-time.Hour)}, sourceKeychain, nil
		}
		// Re-read: Claude Code has since refreshed the Keychain token.
		return Credentials{AccessToken: "kc-fresh", ExpiresAt: now.Add(time.Hour)}, sourceKeychain, nil
	}

	got, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.FiveHour.Utilization != 5 {
		t.Errorf("FiveHour = %v, want 5", got.FiveHour.Utilization)
	}
	if s := atomic.LoadInt32(&saved); s != 0 {
		t.Errorf("saveFile called %d times; Keychain path must never persist", s)
	}
	if c.credsCached.AccessToken != "kc-fresh" {
		t.Errorf("cached AccessToken = %q, want kc-fresh", c.credsCached.AccessToken)
	}
}

// TestClient_Fetch_FileSourceExpired_RefreshesAndPersistsToFile preserves the
// good behavior for file-sourced credentials: clyde and Claude Code share the
// file, so refreshing and writing the rotated token back to the SAME file
// keeps both in sync.
func TestClient_Fetch_FileSourceExpired_RefreshesAndPersistsToFile(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	_, hc := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"file-new-at","refresh_token":"file-new-rt","expires_in":3600}`))
		case "/api/oauth/usage":
			if got := r.Header.Get("Authorization"); got != "Bearer file-new-at" {
				t.Errorf("Authorization = %q, want Bearer file-new-at", got)
			}
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":7},"seven_day":{"utilization":8}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	c := NewClientWithDeps(hc, fakeClock(now))
	var persisted Credentials
	var saved int32
	c.saveFile = func(cr Credentials) error {
		atomic.AddInt32(&saved, 1)
		persisted = cr
		return nil
	}
	c.loadCreds = func() (Credentials, credSource, error) {
		return Credentials{AccessToken: "file-old", RefreshToken: "file-rt", ExpiresAt: now.Add(-time.Hour)}, sourceFile, nil
	}

	got, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.FiveHour.Utilization != 7 {
		t.Errorf("FiveHour = %v, want 7", got.FiveHour.Utilization)
	}
	if s := atomic.LoadInt32(&saved); s != 1 {
		t.Fatalf("saveFile called %d times, want 1 (file source must persist back)", s)
	}
	if persisted.AccessToken != "file-new-at" || persisted.RefreshToken != "file-new-rt" {
		t.Errorf("persisted = {%q,%q}, want {file-new-at, file-new-rt}", persisted.AccessToken, persisted.RefreshToken)
	}
}

// TestClient_Fetch_Keychain401_DeclinesRefresh covers the reactive (post-401)
// path for a Keychain token that is fresh-by-clock but rejected by the server.
// With no newer token available in the Keychain, clyde declines rather than
// rotating the shared refresh token.
func TestClient_Fetch_Keychain401_DeclinesRefresh(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	_, hc := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/usage":
			w.WriteHeader(http.StatusUnauthorized)
		case "/v1/oauth/token":
			t.Errorf("refresh endpoint must NOT be hit for Keychain-sourced creds")
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	c := NewClientWithDeps(hc, fakeClock(now))
	var saved int32
	c.saveFile = func(Credentials) error { atomic.AddInt32(&saved, 1); return nil }
	// Fresh by clock (so no proactive refresh), but the server rejects it, and
	// a re-read returns the SAME token (Claude Code has not refreshed).
	c.loadCreds = func() (Credentials, credSource, error) {
		return Credentials{AccessToken: "kc-tok", ExpiresAt: now.Add(time.Hour)}, sourceKeychain, nil
	}

	_, err := c.Fetch(context.Background())
	if !errors.Is(err, ports.ErrPlanUsageUnavailable) {
		t.Errorf("Keychain 401: err = %v, want ports.ErrPlanUsageUnavailable", err)
	}
	if got := atomic.LoadInt32(&saved); got != 0 {
		t.Errorf("saveFile called %d times; Keychain path must never persist", got)
	}
}

// TestClient_Fetch_Keychain401_PicksUpNewKeychainToken shows the reactive
// path recovering when Claude Code has refreshed the Keychain token between
// clyde's first request and the 401 retry — still without any refresh call.
func TestClient_Fetch_Keychain401_PicksUpNewKeychainToken(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var usageHits int32

	_, hc := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/usage":
			if atomic.AddInt32(&usageHits, 1) == 1 {
				w.WriteHeader(http.StatusUnauthorized) // old token rejected
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer kc-new" {
				t.Errorf("retry Authorization = %q, want Bearer kc-new", got)
			}
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":3},"seven_day":{"utilization":4}}`))
		case "/v1/oauth/token":
			t.Errorf("refresh endpoint must NOT be hit for Keychain-sourced creds")
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	c := NewClientWithDeps(hc, fakeClock(now))
	c.saveFile = func(Credentials) error {
		t.Error("saveFile must not be called for Keychain source")
		return nil
	}
	var reads int32
	c.loadCreds = func() (Credentials, credSource, error) {
		if atomic.AddInt32(&reads, 1) == 1 {
			return Credentials{AccessToken: "kc-old", ExpiresAt: now.Add(time.Hour)}, sourceKeychain, nil
		}
		return Credentials{AccessToken: "kc-new", ExpiresAt: now.Add(time.Hour)}, sourceKeychain, nil
	}

	got, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.FiveHour.Utilization != 3 {
		t.Errorf("FiveHour = %v, want 3", got.FiveHour.Utilization)
	}
	if c.credsCached.AccessToken != "kc-new" {
		t.Errorf("cached AccessToken = %q, want kc-new", c.credsCached.AccessToken)
	}
}

// TestLoadCredentialsWithSource_FileSource verifies provenance reporting for
// the file path. The Keychain reader is faked to miss so the file branch is
// exercised on darwin too — no real Keychain access.
func TestLoadCredentialsWithSource_FileSource(t *testing.T) {
	// Not parallel: mutates HOME and the keychainLoader package var.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	orig := keychainLoader
	keychainLoader = func() (Credentials, error) { return Credentials{}, ErrCredentialsNotFound }
	t.Cleanup(func() { keychainLoader = orig })

	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{"claudeAiOauth":{"accessToken":"file-tok","refreshToken":"rt","expiresAt":1735603200000}}`
	if err := os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	c, src, err := loadCredentialsWithSource()
	if err != nil {
		t.Fatalf("loadCredentialsWithSource: %v", err)
	}
	if src != sourceFile {
		t.Errorf("src = %v, want sourceFile", src)
	}
	if c.AccessToken != "file-tok" {
		t.Errorf("AccessToken = %q, want file-tok", c.AccessToken)
	}
}

// TestLoadCredentialsWithSource_KeychainSource verifies provenance reporting
// for the Keychain path via the fake reader — never touches the real Keychain.
func TestLoadCredentialsWithSource_KeychainSource(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Keychain source only applies on darwin")
	}
	orig := keychainLoader
	keychainLoader = func() (Credentials, error) {
		return Credentials{AccessToken: "kc-tok"}, nil
	}
	t.Cleanup(func() { keychainLoader = orig })

	c, src, err := loadCredentialsWithSource()
	if err != nil {
		t.Fatalf("loadCredentialsWithSource: %v", err)
	}
	if src != sourceKeychain {
		t.Errorf("src = %v, want sourceKeychain", src)
	}
	if c.AccessToken != "kc-tok" {
		t.Errorf("AccessToken = %q, want kc-tok", c.AccessToken)
	}
}

// TestLoadCredentialsWithSource_NotFound: neither source has credentials.
func TestLoadCredentialsWithSource_NotFound(t *testing.T) {
	// Not parallel: mutates HOME and the keychainLoader package var.
	t.Setenv("HOME", t.TempDir()) // empty home, no credentials file
	orig := keychainLoader
	keychainLoader = func() (Credentials, error) { return Credentials{}, ErrCredentialsNotFound }
	t.Cleanup(func() { keychainLoader = orig })

	_, src, err := loadCredentialsWithSource()
	if !errors.Is(err, ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
	if src != sourceNone {
		t.Errorf("src = %v, want sourceNone", src)
	}
}
