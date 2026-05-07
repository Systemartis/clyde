//go:build darwin

package anthropicapi

import (
	"errors"
	"os/exec"
	"testing"
)

// TestLoadFromKeychain_RealBinary exercises the macOS `security` binary
// shellout. The test does NOT depend on Claude Code being installed: it
// just asserts that the call completes and returns either real credentials
// (developer machines that happen to have Claude Code) or the sentinel
// ErrCredentialsNotFound. Anything else means the shellout, the stderr
// matcher, or the context timeout regressed.
//
// Skipped on a runner without /usr/bin/security (vanishingly rare on
// macOS, but covers minimal containers that strip system tools). Pure-
// linux CI never runs this file (//go:build darwin gates the package).
func TestLoadFromKeychain_RealBinary(t *testing.T) {
	if _, err := exec.LookPath("security"); err != nil {
		t.Skipf("security binary not on PATH: %v", err)
	}

	creds, err := loadFromKeychain()

	switch {
	case err == nil:
		// Found creds — sanity-check the shape so a future malformed
		// Keychain payload surfaces here, not in the OAuth refresh path.
		if creds.AccessToken == "" {
			t.Error("Keychain returned no error but AccessToken is empty")
		}
	case errors.Is(err, ErrCredentialsNotFound):
		// Routine "no creds in Keychain on this machine" — that's fine.
	default:
		t.Errorf("loadFromKeychain returned unexpected error: %v", err)
	}
}
