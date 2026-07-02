// Package anthropicapi implements the ports.PlanUsageSource interface by
// calling Anthropic's OAuth-protected /api/oauth/usage endpoint with the
// access token that Claude Code CLI stores locally.
//
// Auth source priority on macOS:
//  1. Keychain service "Claude Code-credentials" (account = current user)
//  2. ~/.claude/.credentials.json (file fallback)
//
// On non-Darwin platforms only the file source is consulted.
//
// The package is intentionally self-contained: no third-party deps, no CGo;
// stdlib net/http and os/exec only.
package anthropicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// keychainStderrCap bounds how much of `security`'s stderr we keep when
// surfacing a Keychain error. The binary's diagnostics are short by
// design; 1 KiB is plenty without risking accidental log bloat.
const keychainStderrCap = 1 << 10

// contextWithTimeout is a thin wrapper that returns a Background-rooted
// timeout context — used by Keychain reads where there's no caller-provided
// context available. Keeps the noctx linter satisfied without forcing every
// caller to thread a context for a 5s shellout.
func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// Credentials is the OAuth state Claude Code persists.
//
// Fields mirror the on-disk schema:
//
//	{
//	  "claudeAiOauth": {
//	    "accessToken":   "sk-ant-oat...",
//	    "refreshToken": "...",
//	    "expiresAt":    1704067200000,        // ms epoch
//	    "scopes":       ["user:profile","user:inference"],
//	    "rateLimitTier": "max_5x"
//	  }
//	}
type Credentials struct {
	AccessToken   string
	RefreshToken  string
	ExpiresAt     time.Time
	Scopes        []string
	RateLimitTier string
}

// Expired reports whether the access token is past its expiry, allowing
// a small skew so we refresh slightly before the actual deadline.
func (c Credentials) Expired(now time.Time) bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	const skew = 30 * time.Second
	return now.Add(skew).After(c.ExpiresAt)
}

// credSource records WHERE a set of credentials was read from. It drives the
// refresh policy: clyde may only rotate + persist tokens it can safely write
// back to the SAME store it read them from.
//
//   - sourceFile: ~/.claude/.credentials.json. clyde and Claude Code share
//     this file, so refreshing and writing the rotated token back keeps both
//     in sync. Refresh + persist is allowed.
//   - sourceKeychain: the macOS Keychain, which clyde reads but cannot safely
//     write (the read never captures the item's full attribute set, and a
//     blind write could create a duplicate item or clobber Claude Code's
//     entry — logging the user out of the CLI). Treated as READ-ONLY: never
//     refresh, only re-read whatever Claude Code has placed there.
//   - sourceNone: no credentials / unknown provenance. Never refresh.
//
// The zero value is sourceNone so that any credentials whose provenance we
// did not explicitly establish are treated conservatively (no refresh).
type credSource int

const (
	sourceNone credSource = iota
	sourceFile
	sourceKeychain
)

// keychainLoader reads credentials from the macOS Keychain. It is a package
// var (not a direct call) so tests can fake the Keychain and never shell out
// to `security` — see requirement that tests must not touch the real Keychain.
var keychainLoader = loadFromKeychain

// LoadCredentials reads OAuth state from Keychain (preferred on macOS) or the
// ~/.claude/.credentials.json file fallback.
//
// Returns ErrCredentialsNotFound when neither source has credentials.
func LoadCredentials() (Credentials, error) {
	c, _, err := loadCredentialsWithSource()
	return c, err
}

// loadCredentialsWithSource is LoadCredentials plus the provenance of the
// credentials it returned, so the refresh path can apply a source-aware,
// safety-first policy (see credSource).
func loadCredentialsWithSource() (Credentials, credSource, error) {
	if runtime.GOOS == "darwin" {
		if c, err := keychainLoader(); err == nil {
			return c, sourceKeychain, nil
		}
		// Fall through to file on Keychain miss / error — Claude Code may
		// have written only one of the two on this machine.
	}
	c, err := loadFromFile(defaultCredentialsPath())
	if err != nil {
		return Credentials{}, sourceNone, err
	}
	return c, sourceFile, nil
}

// defaultCredentialsPath returns ~/.claude/.credentials.json — clyde's only
// supported file location.
func defaultCredentialsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", ".credentials.json")
}

// loadFromKeychain shells out to the macOS `security` binary to read the
// "Claude Code-credentials" generic password and parse it as the same JSON
// blob the file fallback uses.
//
// Errors are typed as ErrCredentialsNotFound when the item is absent so
// callers can fall through to file. All other errors are returned as-is.
func loadFromKeychain() (Credentials, error) {
	// 5s ceiling: the security binary normally returns instantly; a hang
	// here would block the TUI's first render.
	ctx, cancel := contextWithTimeout(5 * time.Second)
	defer cancel()
	// `security find-generic-password -s "Claude Code-credentials" -w`
	// prints just the password (here: the JSON blob) on stdout.
	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-s", "Claude Code-credentials",
		"-w",
	)
	// Capture stderr into a capped buffer so we can surface a useful
	// reason on failure. We intentionally do NOT route it to the parent's
	// stderr — that feed is the bubbletea-rendered terminal and any direct
	// write would corrupt the TUI.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// "item not found" is the routine "no creds in Keychain" path —
		// callers fall through to the file source. Anything else is a
		// real Keychain error worth surfacing.
		stderrText := truncateBytes(stderr.Bytes(), keychainStderrCap)
		if isKeychainItemNotFound(stderrText) {
			return Credentials{}, ErrCredentialsNotFound
		}
		return Credentials{}, fmt.Errorf("%w: %s", ErrCredentialsNotFound, snippet([]byte(stderrText)))
	}
	return parseCredentialsJSON(out)
}

// truncateBytes returns the first n bytes of b as a string, with an
// ellipsis appended when truncation occurred. Bounds the size of stderr
// snippets we propagate into errors / logs.
func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// isKeychainItemNotFound reports whether the security(1) stderr text
// matches the routine "no credential present" outcome. The exact phrasing
// has been stable across macOS releases since at least 10.15.
func isKeychainItemNotFound(stderr string) bool {
	if stderr == "" {
		// Some shells eat the diagnostic; treat empty stderr as "not found"
		// to preserve the file fallback path on machines where Keychain
		// access is denied silently.
		return true
	}
	low := strings.ToLower(stderr)
	return strings.Contains(low, "could not be found") ||
		strings.Contains(low, "errsecitemnotfound") ||
		strings.Contains(low, "specified item could not be found")
}

// loadFromFile reads ~/.claude/.credentials.json, returning
// ErrCredentialsNotFound when the file is absent.
func loadFromFile(path string) (Credentials, error) {
	if path == "" {
		return Credentials{}, ErrCredentialsNotFound
	}
	data, err := os.ReadFile(path) //nolint:gosec // trusted user-controlled path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, ErrCredentialsNotFound
		}
		return Credentials{}, fmt.Errorf("read credentials: %w", err)
	}
	return parseCredentialsJSON(data)
}

// rawCredentialsFile mirrors the JSON shape Claude Code writes.
type rawCredentialsFile struct {
	ClaudeAIOAuth rawOAuth `json:"claudeAiOauth"`
}

type rawOAuth struct {
	AccessToken   string   `json:"accessToken"`
	RefreshToken  string   `json:"refreshToken"`
	ExpiresAtMS   int64    `json:"expiresAt"`
	Scopes        []string `json:"scopes"`
	RateLimitTier string   `json:"rateLimitTier"`
}

// parseCredentialsJSON parses the JSON blob (shared file/Keychain shape).
func parseCredentialsJSON(data []byte) (Credentials, error) {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return Credentials{}, ErrCredentialsNotFound
	}
	var raw rawCredentialsFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return Credentials{}, fmt.Errorf("decode credentials: %w", err)
	}
	if raw.ClaudeAIOAuth.AccessToken == "" {
		return Credentials{}, ErrCredentialsNotFound
	}
	return Credentials{
		AccessToken:   raw.ClaudeAIOAuth.AccessToken,
		RefreshToken:  raw.ClaudeAIOAuth.RefreshToken,
		ExpiresAt:     time.UnixMilli(raw.ClaudeAIOAuth.ExpiresAtMS).UTC(),
		Scopes:        raw.ClaudeAIOAuth.Scopes,
		RateLimitTier: raw.ClaudeAIOAuth.RateLimitTier,
	}, nil
}

// SaveCredentials writes refreshed credentials back to the file location so
// other clyde processes (and a long-running clyde session) can pick up the
// new tokens.
//
// It is ONLY called when the credentials were read from the file in the first
// place (credSource == sourceFile). Keychain is strictly read-only from clyde:
// when tokens originate in the Keychain we never refresh and never write here,
// because writing the file copy would be ineffective (the next read is
// Keychain-first on macOS) and rotating the Keychain-shared refresh token
// could log the user out of the Claude Code CLI. See the refresh policy in
// client.go.
//
// Best-effort: errors from the write are returned so callers can decide
// whether to surface them, but the in-memory Credentials remain valid.
func SaveCredentials(c Credentials) error {
	path := defaultCredentialsPath()
	if path == "" {
		return errors.New("cannot determine credentials path")
	}
	raw := rawCredentialsFile{
		ClaudeAIOAuth: rawOAuth{
			AccessToken:   c.AccessToken,
			RefreshToken:  c.RefreshToken,
			ExpiresAtMS:   c.ExpiresAt.UnixMilli(),
			Scopes:        c.Scopes,
			RateLimitTier: c.RateLimitTier,
		},
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir credentials dir: %w", err)
	}
	// Atomic replace: a crash mid-write (laptop suspend, OOM) on a plain
	// truncate-then-write would leave a half-written credentials file —
	// breaking BOTH clyde and the upstream Claude Code CLI, since both
	// share `~/.claude/.credentials.json`. Write to a sibling tmp first
	// then rename. 0o600: only the owner should read OAuth tokens.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write credentials tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename credentials: %w", err)
	}
	return nil
}

// ErrCredentialsNotFound is returned by loaders when neither source had
// credentials. The package-level Fetch path translates this to
// ports.ErrPlanUsageUnavailable.
var ErrCredentialsNotFound = errors.New("anthropicapi: credentials not found")

// HumanTier maps the raw rateLimitTier string ("max_5x", "pro", "team_max",
// "ultra_pro", ...) into a short human label. Unknown values are
// title-cased verbatim.
//
// Mapping rules (most-specific first):
//   - "max"  or "max_<x>"   → "Max" / "Max <x>"   (preserve the multiplier)
//   - contains "ultra"      → "Ultra"
//   - contains "team"       → "Team"
//   - contains "enterprise" → "Enterprise"
//   - contains "max"        → "Max" (fallback for things like "team_max")
//   - contains "pro"        → "Pro"
func HumanTier(raw string) string {
	if raw == "" {
		return ""
	}
	low := strings.ToLower(raw)
	switch {
	case low == "max":
		return "Max"
	case strings.HasPrefix(low, "max_"):
		return "Max " + strings.TrimPrefix(low, "max_")
	case strings.Contains(low, "ultra"):
		return "Ultra"
	case strings.Contains(low, "team"):
		return "Team"
	case strings.Contains(low, "enterprise"):
		return "Enterprise"
	case strings.Contains(low, "max"):
		return "Max"
	case strings.Contains(low, "pro"):
		return "Pro"
	}
	// Fallback: title-case the raw value for display.
	return strings.ToUpper(raw[:1]) + raw[1:]
}
