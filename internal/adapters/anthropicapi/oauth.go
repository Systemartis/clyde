package anthropicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Public PKCE client_id used by Claude Code's OAuth flow. This is not a
// secret — it identifies the public client. Pinned here so the refresh
// endpoint accepts our request.
const oauthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

// Refresh endpoint host. Note: NOT api.anthropic.com.
const oauthRefreshURL = "https://platform.claude.com/v1/oauth/token"

// ErrInvalidGrant is the terminal refresh failure: the refresh_token has
// been revoked or expired beyond recovery. Translated to
// ports.ErrPlanUsageAuth at the package boundary.
var ErrInvalidGrant = errors.New("anthropicapi: invalid_grant — re-authentication required")

// refreshTokens exchanges a refresh token for a new access+refresh pair.
// Returns updated Credentials with the new tokens and expiry; the rest of
// the input fields (Scopes, RateLimitTier) are preserved.
func refreshTokens(ctx context.Context, httpClient *http.Client, current Credentials) (Credentials, error) {
	if current.RefreshToken == "" {
		return Credentials{}, ErrInvalidGrant
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {current.RefreshToken},
		"client_id":     {oauthClientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthRefreshURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return Credentials{}, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return Credentials{}, fmt.Errorf("post refresh: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		// Detect the terminal invalid_grant error so callers can stop
		// retrying and surface a re-auth prompt.
		if isInvalidGrant(body) {
			return Credentials{}, ErrInvalidGrant
		}
		return Credentials{}, fmt.Errorf("refresh returned %d: %s", resp.StatusCode, snippet(body))
	}

	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"` // seconds
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return Credentials{}, fmt.Errorf("decode refresh response: %w", err)
	}
	if out.AccessToken == "" {
		return Credentials{}, errors.New("refresh response missing access_token")
	}

	updated := current
	updated.AccessToken = out.AccessToken
	if out.RefreshToken != "" {
		updated.RefreshToken = out.RefreshToken
	}
	if out.ExpiresIn > 0 {
		updated.ExpiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second).UTC()
	}
	return updated, nil
}

// isInvalidGrant inspects a refresh failure body for the OAuth-standard
// invalid_grant error code.
func isInvalidGrant(body []byte) bool {
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return false
	}
	return e.Error == "invalid_grant"
}

// tokenLikePattern matches runs of 32+ characters drawn from the
// alphabet OAuth tokens use (URL-safe base64-ish). Used by snippet to
// redact bearer / refresh / authorization-code-shaped substrings before
// embedding response bodies in error messages — those errors can land
// in user-visible logs.
var tokenLikePattern = regexp.MustCompile(`[A-Za-z0-9_\-]{32,}`)

// snippet truncates a body for safe inclusion in error messages and
// scrubs anything that looks like a token. The Anthropic OAuth flow's
// error responses (e.g. `{"error":"invalid_grant"}`) survive — we only
// redact long opaque strings.
func snippet(b []byte) string {
	const max = 200
	s := string(b)
	if len(s) > max {
		s = s[:max] + "…"
	}
	return tokenLikePattern.ReplaceAllString(s, "<redacted>")
}
