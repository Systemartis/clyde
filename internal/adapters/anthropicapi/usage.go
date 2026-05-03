package anthropicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// usageEndpoint is the OAuth-protected endpoint that returns plan-quota
// usage — the same data shown on https://claude.ai/settings/usage.
const usageEndpoint = "https://api.anthropic.com/api/oauth/usage"

// Mandatory beta header. The endpoint refuses requests that omit it.
const oauthBetaHeader = "oauth-2025-04-20"

// userAgent identifies the client to Anthropic. Mirrors Claude Code's
// pattern so our requests look like a normal CLI client. Versioned
// because API behavior MAY differ on Claude Code version.
const userAgent = "claude-code/2.1.0 (clyde)"

// errAccessTokenExpired is the typed sentinel returned when the API
// answers 401 — the caller refreshes and retries.
var errAccessTokenExpired = errors.New("anthropicapi: access token expired")

// fetchUsage performs a single GET against /api/oauth/usage with the supplied
// access token. Callers handle the 401→refresh→retry dance; this function
// just makes one request and decodes the response.
//
// Returns the typed body alongside a presence map so callers can distinguish
// "window absent from response" (Present=false) from "window present, 0%".
func fetchUsage(ctx context.Context, httpClient *http.Client, accessToken string) (rawUsageResponse, map[string]bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageEndpoint, nil)
	if err != nil {
		return rawUsageResponse{}, nil, fmt.Errorf("build usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", oauthBetaHeader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return rawUsageResponse{}, nil, fmt.Errorf("get usage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusUnauthorized:
		return rawUsageResponse{}, nil, errAccessTokenExpired
	default:
		return rawUsageResponse{}, nil, fmt.Errorf("usage returned %d: %s", resp.StatusCode, snippet(body))
	}

	return decodeEnvelope(body)
}

// rawUsageResponse is the JSON shape returned by /api/oauth/usage. Only the
// windows clyde surfaces are decoded — Sonnet-only, Opus-only, Design,
// Routines, and ExtraUsage are intentionally ignored per scoping decision.
type rawUsageResponse struct {
	FiveHour rawWindow `json:"five_hour"`
	SevenDay rawWindow `json:"seven_day"`
}

// rawWindow is one quota window. utilization is decoded via flexible
// helper because the API returns it as either int or float depending on
// the value.
type rawWindow struct {
	Utilization flexFloat `json:"utilization"`
	ResetsAt    string    `json:"resets_at"`
	// Present is set in unmarshalToPlanWindow — JSON unmarshal alone
	// cannot distinguish "key absent" from "key = zero" in a Go struct,
	// so the wrapper response decoder handles that bookkeeping.
}

// flexFloat decodes a JSON number that the API returns as either int
// (e.g. 30) or float (e.g. 12.5).
type flexFloat float64

// UnmarshalJSON accepts both numeric forms.
func (f *flexFloat) UnmarshalJSON(b []byte) error {
	var n float64
	if err := json.Unmarshal(b, &n); err == nil {
		*f = flexFloat(n)
		return nil
	}
	// Try string fallback ("30" — defensive).
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	var n2 float64
	if _, err := fmt.Sscanf(s, "%f", &n2); err != nil {
		return fmt.Errorf("flexFloat: cannot parse %q: %w", s, err)
	}
	*f = flexFloat(n2)
	return nil
}

// decodeEnvelope splits the body into a typed view AND a key-presence map.
// Presence detection is needed because JSON unmarshal alone cannot tell us
// whether a window was absent vs. zero — and Anthropic A/B tests rename
// these keys, so we don't want to draw a fake 0% bar for a missing window.
func decodeEnvelope(body []byte) (rawUsageResponse, map[string]bool, error) {
	var typed rawUsageResponse
	if err := json.Unmarshal(body, &typed); err != nil {
		return rawUsageResponse{}, nil, fmt.Errorf("decode usage response: %w", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		return rawUsageResponse{}, nil, fmt.Errorf("decode usage envelope: %w", err)
	}
	present := map[string]bool{
		"five_hour": isPresent(keys["five_hour"]),
		"seven_day": isPresent(keys["seven_day"]),
	}
	return typed, present, nil
}

// isPresent returns true when the JSON value is a non-null object.
func isPresent(raw json.RawMessage) bool {
	s := string(raw)
	return s != "" && s != "null"
}

// parseResetsAt parses an RFC3339 timestamp; returns zero on parse failure.
func parseResetsAt(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
