package anthropicapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRefreshTokens_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form := string(body)
		// Verify the form fields the OAuth spec requires.
		for _, want := range []string{"grant_type=refresh_token", "refresh_token=old-rt", "client_id="} {
			if !strings.Contains(form, want) {
				t.Errorf("refresh body missing %q; got %s", want, form)
			}
		}
		_, _ = w.Write([]byte(`{
			"access_token":  "new-at",
			"refresh_token": "new-rt",
			"expires_in":    3600,
			"token_type":    "Bearer"
		}`))
	}))
	t.Cleanup(srv.Close)

	httpClient := srv.Client()
	httpClient.Transport = &rewriteTransport{base: httpClient.Transport, target: srv.URL}

	current := Credentials{
		AccessToken:  "old-at",
		RefreshToken: "old-rt",
		ExpiresAt:    time.Now().Add(-time.Hour).UTC(),
		Scopes:       []string{"user:profile"},
	}
	updated, err := refreshTokens(context.Background(), httpClient, current)
	if err != nil {
		t.Fatalf("refreshTokens: %v", err)
	}
	if updated.AccessToken != "new-at" {
		t.Errorf("AccessToken = %q, want new-at", updated.AccessToken)
	}
	if updated.RefreshToken != "new-rt" {
		t.Errorf("RefreshToken = %q, want new-rt", updated.RefreshToken)
	}
	if !updated.ExpiresAt.After(time.Now()) {
		t.Errorf("ExpiresAt should be in future, got %v", updated.ExpiresAt)
	}
	// Scopes preserved from input.
	if len(updated.Scopes) != 1 || updated.Scopes[0] != "user:profile" {
		t.Errorf("Scopes not preserved: %v", updated.Scopes)
	}
}

func TestRefreshTokens_InvalidGrant(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token revoked"}`))
	}))
	t.Cleanup(srv.Close)

	httpClient := srv.Client()
	httpClient.Transport = &rewriteTransport{base: httpClient.Transport, target: srv.URL}

	_, err := refreshTokens(context.Background(), httpClient,
		Credentials{RefreshToken: "rt"})
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("invalid_grant: err = %v, want ErrInvalidGrant", err)
	}
}

func TestRefreshTokens_NoRefreshToken(t *testing.T) {
	t.Parallel()

	_, err := refreshTokens(context.Background(), http.DefaultClient,
		Credentials{}) // no refresh token
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("missing refresh: err = %v, want ErrInvalidGrant", err)
	}
}

func TestRefreshTokens_OtherError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`Service Unavailable`))
	}))
	t.Cleanup(srv.Close)

	httpClient := srv.Client()
	httpClient.Transport = &rewriteTransport{base: httpClient.Transport, target: srv.URL}

	_, err := refreshTokens(context.Background(), httpClient,
		Credentials{RefreshToken: "rt"})
	if err == nil {
		t.Fatal("5xx: want error")
	}
	if errors.Is(err, ErrInvalidGrant) {
		t.Error("5xx must NOT classify as invalid_grant")
	}
}

// TestSnippet_RedactsTokenLikeStrings — long opaque strings (anything
// 32+ chars from the bearer-token alphabet) get scrubbed before they
// can land in an error message that might be logged. Short structured
// content like JSON error names survives unchanged.
func TestSnippet_RedactsTokenLikeStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"short error name preserved",
			`{"error":"invalid_grant"}`,
			`{"error":"invalid_grant"}`,
		},
		{
			"long alphanum substring redacted",
			`access_token=sk_live_aBc123XYZ456deF789ghIjKlMnOpQrStUvWxYz_more_stuff_too`,
			`access_token=<redacted>`,
		},
		{
			"short alphanum (under 32) preserved",
			`code=abc123def`,
			`code=abc123def`,
		},
		{
			"bearer JWT-shaped — header + signature segments redacted (short payload survives)",
			`Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c`,
			`Bearer <redacted>.eyJzdWIiOiIxMjM0NTY3ODkwIn0.<redacted>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := snippet([]byte(tc.in))
			if got != tc.want {
				t.Errorf("snippet(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}
