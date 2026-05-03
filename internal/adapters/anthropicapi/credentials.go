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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

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

// LoadCredentials reads OAuth state from Keychain (preferred on macOS) or the
// ~/.claude/.credentials.json file fallback.
//
// Returns ports.ErrPlanUsageUnavailable when neither source has credentials.
func LoadCredentials() (Credentials, error) {
	if runtime.GOOS == "darwin" {
		if c, err := loadFromKeychain(); err == nil {
			return c, nil
		}
		// Fall through to file on Keychain miss / error — Claude Code may
		// have written only one of the two on this machine.
	}
	c, err := loadFromFile(defaultCredentialsPath())
	if err != nil {
		return Credentials{}, err
	}
	return c, nil
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
	// Suppress stderr — the parent process's stderr feeds into the
	// bubbletea-rendered terminal, and any diagnostic from `security`
	// (e.g. user-not-authenticated prompts) would corrupt the TUI.
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return Credentials{}, ErrCredentialsNotFound
	}
	return parseCredentialsJSON(out)
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
// new tokens. Keychain is read-only from clyde; we do not touch it.
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
