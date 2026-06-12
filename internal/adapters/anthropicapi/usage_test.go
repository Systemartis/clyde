package anthropicapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFlexFloat_AcceptsIntAndFloat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		body string
		want float64
	}{
		{`{"v": 30}`, 30},
		{`{"v": 12.5}`, 12.5},
		{`{"v": 0}`, 0},
		{`{"v": "30"}`, 30}, // string fallback
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			t.Parallel()
			var w struct {
				V flexFloat `json:"v"`
			}
			if err := unmarshalJSON([]byte(tc.body), &w); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if float64(w.V) != tc.want {
				t.Errorf("flexFloat(%q) = %v, want %v", tc.body, w.V, tc.want)
			}
		})
	}
}

func TestDecodeEnvelope_PresenceTracking(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"five_hour":  {"utilization": 12.5, "resets_at": "2026-01-01T12:00:00Z"},
		"seven_day":  {"utilization": 30}
	}`)
	typed, present, err := decodeEnvelope(body)
	if err != nil {
		t.Fatalf("decodeEnvelope: %v", err)
	}
	if !present["five_hour"] {
		t.Error("five_hour should be Present=true")
	}
	if !present["seven_day"] {
		t.Error("seven_day should be Present=true")
	}
	if float64(typed.FiveHour.Utilization) != 12.5 {
		t.Errorf("FiveHour.Utilization = %v, want 12.5", typed.FiveHour.Utilization)
	}
	if typed.FiveHour.ResetsAt != "2026-01-01T12:00:00Z" {
		t.Errorf("FiveHour.ResetsAt = %q, want 2026-01-01T12:00:00Z", typed.FiveHour.ResetsAt)
	}
}

func TestDecodeEnvelope_AbsentKeys(t *testing.T) {
	t.Parallel()

	body := []byte(`{"five_hour": {"utilization": 50}}`)
	_, present, err := decodeEnvelope(body)
	if err != nil {
		t.Fatalf("decodeEnvelope: %v", err)
	}
	if !present["five_hour"] {
		t.Error("five_hour should be Present=true")
	}
	if present["seven_day"] {
		t.Error("seven_day should be Present=false (key absent)")
	}
}

func TestFetchUsage_SendsRequiredHeaders(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify required headers.
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want Bearer tok", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != oauthBetaHeader {
			t.Errorf("anthropic-beta = %q, want %q", got, oauthBetaHeader)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		// Echo a valid body.
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":42},"seven_day":{"utilization":80}}`))
	}))
	t.Cleanup(srv.Close)

	// Override the endpoint by directing the request through srv.URL.
	// fetchUsage hardcodes the URL, so we use a transport-level rewrite via the test server's client.
	httpClient := srv.Client()
	httpClient.Transport = &rewriteTransport{base: httpClient.Transport, target: srv.URL}

	raw, present, err := fetchUsage(context.Background(), httpClient, "tok")
	if err != nil {
		t.Fatalf("fetchUsage: %v", err)
	}
	if float64(raw.FiveHour.Utilization) != 42 {
		t.Errorf("FiveHour = %v, want 42", raw.FiveHour.Utilization)
	}
	if !present["five_hour"] {
		t.Error("five_hour should be present")
	}
}

func TestFetchUsage_401(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	t.Cleanup(srv.Close)

	httpClient := srv.Client()
	httpClient.Transport = &rewriteTransport{base: httpClient.Transport, target: srv.URL}

	if _, _, err := fetchUsage(context.Background(), httpClient, "tok"); !errors.Is(err, errAccessTokenExpired) {
		t.Errorf("401: err = %v, want errAccessTokenExpired", err)
	}
}

func TestFetchUsage_5xxReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server exploded`))
	}))
	t.Cleanup(srv.Close)

	httpClient := srv.Client()
	httpClient.Transport = &rewriteTransport{base: httpClient.Transport, target: srv.URL}

	_, _, err := fetchUsage(context.Background(), httpClient, "tok")
	if err == nil {
		t.Fatal("5xx: want error")
	}
	if errors.Is(err, errAccessTokenExpired) {
		t.Error("5xx: should not be classified as expired-token")
	}
}

// rewriteTransport rewrites every outgoing request URL to hit `target`,
// preserving the path so we can intercept hardcoded endpoints in tests.
type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	u := req2.URL
	// Replace scheme+host with target's. The path is preserved.
	target, err := http.NewRequestWithContext(req2.Context(), req2.Method, r.target+u.Path, nil)
	if err != nil {
		return nil, err
	}
	req2.URL = target.URL
	req2.Host = target.URL.Host
	if r.base == nil {
		return http.DefaultTransport.RoundTrip(req2)
	}
	return r.base.RoundTrip(req2)
}

// unmarshalJSON aliases json.Unmarshal so flexFloat tests stay terse.
func unmarshalJSON(data []byte, v any) error { return json.Unmarshal(data, v) }
