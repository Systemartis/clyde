package anthropicapi

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCredentialsJSON_Valid(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"claudeAiOauth": {
			"accessToken":   "sk-ant-oat-abc",
			"refreshToken":  "rt-xyz",
			"expiresAt":     1735603200000,
			"scopes":        ["user:profile", "user:inference"],
			"rateLimitTier": "max_5x"
		}
	}`)

	c, err := parseCredentialsJSON(body)
	if err != nil {
		t.Fatalf("parseCredentialsJSON: %v", err)
	}
	if c.AccessToken != "sk-ant-oat-abc" {
		t.Errorf("AccessToken = %q, want sk-ant-oat-abc", c.AccessToken)
	}
	if c.RefreshToken != "rt-xyz" {
		t.Errorf("RefreshToken = %q, want rt-xyz", c.RefreshToken)
	}
	if c.RateLimitTier != "max_5x" {
		t.Errorf("RateLimitTier = %q, want max_5x", c.RateLimitTier)
	}
	want := time.UnixMilli(1735603200000).UTC()
	if !c.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", c.ExpiresAt, want)
	}
	if len(c.Scopes) != 2 {
		t.Errorf("Scopes len = %d, want 2", len(c.Scopes))
	}
}

func TestParseCredentialsJSON_Empty(t *testing.T) {
	t.Parallel()

	if _, err := parseCredentialsJSON([]byte{}); !errors.Is(err, ErrCredentialsNotFound) {
		t.Errorf("empty body: err = %v, want ErrCredentialsNotFound", err)
	}
	if _, err := parseCredentialsJSON([]byte("   ")); !errors.Is(err, ErrCredentialsNotFound) {
		t.Errorf("whitespace body: err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestParseCredentialsJSON_MissingAccessToken(t *testing.T) {
	t.Parallel()

	body := []byte(`{"claudeAiOauth":{"refreshToken":"rt"}}`)
	if _, err := parseCredentialsJSON(body); !errors.Is(err, ErrCredentialsNotFound) {
		t.Errorf("missing accessToken: err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestExpired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"zero — never expires", time.Time{}, false},
		{"future", now.Add(time.Hour), false},
		{"past", now.Add(-time.Hour), true},
		// 30s skew window — token expiring in 5s is treated as expired.
		{"within skew", now.Add(5 * time.Second), true},
		{"just outside skew", now.Add(31 * time.Second), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := Credentials{ExpiresAt: tc.expiresAt}
			if got := c.Expired(now); got != tc.want {
				t.Errorf("Expired(%v, now=%v) = %v, want %v", tc.expiresAt, now, got, tc.want)
			}
		})
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	if _, err := loadFromFile(path); !errors.Is(err, ErrCredentialsNotFound) {
		t.Errorf("missing file: err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestLoadFromFile_Roundtrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials.json")
	body := `{
		"claudeAiOauth": {
			"accessToken": "tok",
			"refreshToken": "rt",
			"expiresAt": 1735603200000,
			"rateLimitTier": "pro"
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c, err := loadFromFile(path)
	if err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	if c.AccessToken != "tok" {
		t.Errorf("AccessToken = %q, want tok", c.AccessToken)
	}
	if c.RateLimitTier != "pro" {
		t.Errorf("RateLimitTier = %q, want pro", c.RateLimitTier)
	}
}

func TestHumanTier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want string
	}{
		{"", ""},
		{"max_5x", "Max 5x"},
		{"max_20x", "Max 20x"},
		{"max", "Max"},
		{"pro", "Pro"},
		{"team_max", "Team"}, // "team" is more meaningful than "max" here
		{"enterprise", "Enterprise"},
		{"ultra_pro", "Ultra"}, // ultra wins over pro
		{"weird-name", "Weird-name"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			if got := HumanTier(tc.raw); got != tc.want {
				t.Errorf("HumanTier(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestSaveCredentials_WritesValidJSON(t *testing.T) {
	// Cannot run in parallel — mutates HOME.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	c := Credentials{
		AccessToken:   "tok",
		RefreshToken:  "rt",
		ExpiresAt:     time.UnixMilli(1735603200000).UTC(),
		Scopes:        []string{"user:profile"},
		RateLimitTier: "max_5x",
	}
	if err := SaveCredentials(c); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	path := filepath.Join(dir, ".claude", ".credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(data), `"accessToken": "tok"`) {
		t.Errorf("output missing accessToken; got: %s", data)
	}

	// Roundtrip via loadFromFile.
	loaded, err := loadFromFile(path)
	if err != nil {
		t.Fatalf("loadFromFile after save: %v", err)
	}
	if loaded.AccessToken != c.AccessToken {
		t.Errorf("AccessToken roundtrip = %q, want %q", loaded.AccessToken, c.AccessToken)
	}
	if !loaded.ExpiresAt.Equal(c.ExpiresAt) {
		t.Errorf("ExpiresAt roundtrip = %v, want %v", loaded.ExpiresAt, c.ExpiresAt)
	}

	// Atomic-write contract: no leftover .tmp file after a successful save.
	// A leftover would mean we either failed to rename or aren't cleaning
	// up — both indicators that the next call could see a half-state.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("leftover tmp file after SaveCredentials: stat err = %v (want IsNotExist)", err)
	}
}

func TestTruncateBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"under cap", "short", 100, "short"},
		{"at cap", "exact", 5, "exact"},
		{"over cap", "abcdefgh", 4, "abcd…"},
		{"empty", "", 10, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateBytes([]byte(tc.in), tc.n)
			if got != tc.want {
				t.Errorf("truncateBytes(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

func TestIsKeychainItemNotFound(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stderr string
		want   bool
	}{
		{"", true},
		{"security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.", true},
		{"errSecItemNotFound", true},
		{"could not be found", true},
		{"User interaction is not allowed.", false},
		{"unrelated error message", false},
	}
	for _, tc := range cases {
		t.Run(tc.stderr, func(t *testing.T) {
			if got := isKeychainItemNotFound(tc.stderr); got != tc.want {
				t.Errorf("isKeychainItemNotFound(%q) = %v, want %v", tc.stderr, got, tc.want)
			}
		})
	}
}
