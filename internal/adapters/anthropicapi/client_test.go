package anthropicapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clyde-tui/clyde/internal/ports"
)

// fakeClock returns a fixed time for deterministic tests.
func fakeClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// newServer wires a single httptest server that routes by path so we can
// model both the usage endpoint and the OAuth refresh endpoint behind one
// rewriteTransport instance.
func newServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := srv.Client()
	c.Transport = &rewriteTransport{base: c.Transport, target: srv.URL}
	return srv, c
}

func TestClient_Fetch_HappyPath(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_, hc := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth/usage" {
			_, _ = w.Write([]byte(`{
				"five_hour":  {"utilization": 49,   "resets_at": "2026-01-01T13:31:00Z"},
				"seven_day":  {"utilization": 79.5, "resets_at": "2026-01-03T19:00:00Z"}
			}`))
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	})

	c := NewClientWithDeps(hc, fakeClock(now))
	c.credsLoaded = true
	c.credsCached = Credentials{
		AccessToken:   "valid",
		ExpiresAt:     now.Add(time.Hour),
		RateLimitTier: "max_5x",
	}

	got, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.FiveHour.Utilization != 49 {
		t.Errorf("FiveHour.Utilization = %v, want 49", got.FiveHour.Utilization)
	}
	if got.SevenDay.Utilization != 79.5 {
		t.Errorf("SevenDay.Utilization = %v, want 79.5", got.SevenDay.Utilization)
	}
	if !got.FiveHour.Present {
		t.Error("FiveHour.Present should be true")
	}
	if got.Tier != "Max 5x" {
		t.Errorf("Tier = %q, want Max 5x", got.Tier)
	}
	if !got.FetchedAt.Equal(now) {
		t.Errorf("FetchedAt = %v, want %v", got.FetchedAt, now)
	}
}

func TestClient_Fetch_RefreshOn401(t *testing.T) {
	t.Parallel()

	var hits int32
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	_, hc := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/usage":
			n := atomic.AddInt32(&hits, 1)
			if n == 1 {
				// First call: token expired.
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			// Second call (after refresh): success.
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":10},"seven_day":{"utilization":20}}`))
		case "/v1/oauth/token":
			body := make([]byte, 1024)
			n, _ := r.Body.Read(body)
			form := string(body[:n])
			if !strings.Contains(form, "refresh_token=rt-old") {
				t.Errorf("refresh body missing rt-old: %s", form)
			}
			_, _ = w.Write([]byte(`{
				"access_token":  "rt-new-at",
				"refresh_token": "rt-new",
				"expires_in":    3600
			}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})

	c := NewClientWithDeps(hc, fakeClock(now))
	c.credsLoaded = true
	c.credsCached = Credentials{
		AccessToken:  "expired-but-not-yet-known",
		RefreshToken: "rt-old",
		ExpiresAt:    now.Add(time.Hour), // not expired by clock; only the API knows
	}

	got, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.FiveHour.Utilization != 10 {
		t.Errorf("FiveHour = %v, want 10", got.FiveHour.Utilization)
	}
	if c.credsCached.AccessToken != "rt-new-at" {
		t.Errorf("cached AccessToken = %q, want rt-new-at", c.credsCached.AccessToken)
	}
	if c.credsCached.RefreshToken != "rt-new" {
		t.Errorf("cached RefreshToken = %q, want rt-new", c.credsCached.RefreshToken)
	}
}

func TestClient_Fetch_ProactiveRefreshWhenExpired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var refreshCalled, usageCalled bool

	_, hc := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth/token":
			refreshCalled = true
			_, _ = w.Write([]byte(`{"access_token":"new","refresh_token":"newrt","expires_in":3600}`))
		case "/api/oauth/usage":
			usageCalled = true
			if got := r.Header.Get("Authorization"); got != "Bearer new" {
				t.Errorf("Authorization = %q, want Bearer new (post-refresh)", got)
			}
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":1},"seven_day":{"utilization":2}}`))
		}
	})

	c := NewClientWithDeps(hc, fakeClock(now))
	c.credsLoaded = true
	c.credsCached = Credentials{
		AccessToken:  "stale",
		RefreshToken: "rt",
		ExpiresAt:    now.Add(-time.Minute), // already expired
	}

	if _, err := c.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !refreshCalled {
		t.Error("refresh endpoint not hit despite expired token")
	}
	if !usageCalled {
		t.Error("usage endpoint not hit")
	}
}

func TestClient_Fetch_InvalidGrantSurfacesAuthError(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	_, hc := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth/token" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		}
	})

	c := NewClientWithDeps(hc, fakeClock(now))
	c.credsLoaded = true
	c.credsCached = Credentials{
		AccessToken:  "stale",
		RefreshToken: "revoked",
		ExpiresAt:    now.Add(-time.Hour),
	}

	_, err := c.Fetch(context.Background())
	if !errors.Is(err, ports.ErrPlanUsageAuth) {
		t.Errorf("invalid_grant: err = %v, want ports.ErrPlanUsageAuth", err)
	}
}

func TestClient_Fetch_NoCredsTranslated(t *testing.T) {
	// Cannot run in parallel — mutates HOME.
	t.Setenv("HOME", t.TempDir()) // empty home; no credentials file

	c := NewClient()
	_, err := c.Fetch(context.Background())
	if !errors.Is(err, ports.ErrPlanUsageUnavailable) {
		t.Errorf("no creds: err = %v, want ports.ErrPlanUsageUnavailable", err)
	}
}
