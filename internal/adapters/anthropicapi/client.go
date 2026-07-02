package anthropicapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Systemartis/clyde/internal/ports"
)

// Client implements ports.PlanUsageSource by combining:
//   - Credentials loading (file + Keychain)
//   - OAuth refresh when the access token is expired or rejected
//   - GET /api/oauth/usage with the Bearer token
//
// A single Client instance is safe for concurrent use; the credentials
// cache + refresh path are guarded by a mutex.
type Client struct {
	httpClient *http.Client

	// now is the time source. Override in tests via NewClientWithClock.
	now func() time.Time

	// loadCreds loads credentials and reports their provenance. It is a seam
	// (not a direct call to loadCredentialsWithSource) so tests can supply
	// fake sources without touching the real Keychain or file.
	loadCreds func() (Credentials, credSource, error)

	// saveFile persists refreshed credentials back to the file. It is a seam
	// so tests never write the real ~/.claude/.credentials.json. It is only
	// invoked for file-sourced credentials (see the refresh policy below).
	saveFile func(Credentials) error

	mu          sync.Mutex
	credsCached Credentials // last-known good credentials (in-memory cache)
	credsLoaded bool        // becomes true after first successful load
	credsSource credSource  // where credsCached was read from
}

// compile-time interface assertion.
var _ ports.PlanUsageSource = (*Client)(nil)

// NewClient returns a Client with sensible defaults — a 10s timeout HTTP
// client and the system clock.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		now:        func() time.Time { return time.Now().UTC() },
		loadCreds:  loadCredentialsWithSource,
		saveFile:   SaveCredentials,
	}
}

// NewClientWithDeps returns a Client with custom HTTP client + clock — the
// hook tests use to inject httptest.Server URLs and a deterministic clock.
func NewClientWithDeps(httpClient *http.Client, now func() time.Time) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Client{
		httpClient: httpClient,
		now:        now,
		loadCreds:  loadCredentialsWithSource,
		saveFile:   SaveCredentials,
	}
}

// Fetch implements ports.PlanUsageSource. It returns the current plan-quota
// snapshot, refreshing the access token transparently if needed.
//
// Translation rules (adapter → port):
//   - ErrCredentialsNotFound → ports.ErrPlanUsageUnavailable
//   - ErrInvalidGrant        → ports.ErrPlanUsageAuth
//   - HTTP 401 after refresh → ports.ErrPlanUsageAuth
//   - everything else        → wrapped raw error
func (c *Client) Fetch(ctx context.Context) (ports.PlanUsage, error) {
	creds, err := c.ensureCredentials(ctx)
	if err != nil {
		return ports.PlanUsage{}, err
	}

	// First attempt with the current access token.
	raw, present, err := fetchUsage(ctx, c.httpClient, creds.AccessToken)
	if errors.Is(err, errAccessTokenExpired) {
		// 401 — refresh and retry once.
		creds, err = c.refreshAndStore(ctx, creds)
		if err != nil {
			return ports.PlanUsage{}, err
		}
		raw, present, err = fetchUsage(ctx, c.httpClient, creds.AccessToken)
	}
	if err != nil {
		if errors.Is(err, errAccessTokenExpired) {
			// Still 401 after fresh token — terminal auth failure.
			return ports.PlanUsage{}, ports.ErrPlanUsageAuth
		}
		return ports.PlanUsage{}, err
	}

	return c.toPortUsage(raw, present, creds), nil
}

// ensureCredentials returns the in-memory credentials, loading and proactively
// refreshing them when expired. Caller takes c.mu via the wrappers.
func (c *Client) ensureCredentials(ctx context.Context) (Credentials, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.credsLoaded {
		loaded, src, err := c.loadCreds()
		if err != nil {
			if errors.Is(err, ErrCredentialsNotFound) {
				return Credentials{}, ports.ErrPlanUsageUnavailable
			}
			return Credentials{}, fmt.Errorf("load credentials: %w", err)
		}
		c.credsCached = loaded
		c.credsSource = src
		c.credsLoaded = true
	}

	if !c.credsCached.Expired(c.now()) {
		return c.credsCached, nil
	}

	// Token expired. How we recover depends on WHERE it came from — the
	// overriding rule is that clyde must never put the user's Claude Code
	// login at risk.
	if c.credsSource != sourceFile {
		// Keychain (or unknown) source: clyde is a READ-ONLY consumer. A
		// refresh would rotate the refresh token that Claude Code shares via
		// the Keychain, and clyde has no safe, verifiable way to write the
		// rotated token back — doing so could invalidate the Keychain token
		// Claude Code depends on and log the user out of the CLI. Instead we
		// re-read: Claude Code keeps its own Keychain token fresh, so pick
		// that up if it has been refreshed; otherwise fall back to the
		// graceful "plan offline" state until it is.
		if reloaded, src, err := c.loadCreds(); err == nil &&
			src == c.credsSource && !reloaded.Expired(c.now()) {
			c.credsCached = reloaded
			return c.credsCached, nil
		}
		return Credentials{}, ports.ErrPlanUsageUnavailable
	}

	// File source: clyde and Claude Code share the file, so refreshing and
	// writing the rotated token back to the SAME file keeps both in sync.
	refreshed, err := refreshTokens(ctx, c.httpClient, c.credsCached)
	if err != nil {
		if errors.Is(err, ErrInvalidGrant) {
			return Credentials{}, ports.ErrPlanUsageAuth
		}
		return Credentials{}, fmt.Errorf("refresh token: %w", err)
	}
	c.credsCached = refreshed
	// Best-effort persist back to the same store it was read from.
	_ = c.saveFile(refreshed)
	return c.credsCached, nil
}

// refreshAndStore is the post-401 retry path: recover fresh credentials and
// update the cache. It applies the same source-aware safety policy as
// ensureCredentials — a Keychain-sourced token is never rotated by clyde.
func (c *Client) refreshAndStore(ctx context.Context, current Credentials) (Credentials, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.credsSource != sourceFile {
		// Keychain (or unknown) source: never rotate the shared refresh
		// token. Try to pick up a token Claude Code refreshed into the
		// Keychain; if there is no newer one, decline gracefully rather than
		// refresh (see ensureCredentials for the full rationale).
		if reloaded, src, err := c.loadCreds(); err == nil &&
			src == c.credsSource && reloaded.AccessToken != current.AccessToken {
			c.credsCached = reloaded
			return reloaded, nil
		}
		return Credentials{}, ports.ErrPlanUsageUnavailable
	}

	refreshed, err := refreshTokens(ctx, c.httpClient, current)
	if err != nil {
		if errors.Is(err, ErrInvalidGrant) {
			return Credentials{}, ports.ErrPlanUsageAuth
		}
		return Credentials{}, fmt.Errorf("refresh token: %w", err)
	}
	c.credsCached = refreshed
	_ = c.saveFile(refreshed)
	return refreshed, nil
}

// toPortUsage converts the raw decoded body into the port DTO.
func (c *Client) toPortUsage(raw rawUsageResponse, present map[string]bool, creds Credentials) ports.PlanUsage {
	return ports.PlanUsage{
		FiveHour: ports.PlanWindow{
			Utilization: float64(raw.FiveHour.Utilization),
			ResetsAt:    parseResetsAt(raw.FiveHour.ResetsAt),
			Present:     present["five_hour"],
		},
		SevenDay: ports.PlanWindow{
			Utilization: float64(raw.SevenDay.Utilization),
			ResetsAt:    parseResetsAt(raw.SevenDay.ResetsAt),
			Present:     present["seven_day"],
		},
		Tier:      HumanTier(creds.RateLimitTier),
		FetchedAt: c.now(),
	}
}
