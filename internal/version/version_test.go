package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestInfo_VersionAlwaysSet(t *testing.T) {
	info := Info()
	if info.Version == "" {
		t.Error("Version must be non-empty (ldflags, BuildInfo, or 'dev' fallback)")
	}
}

func TestInfo_GoVersionFromRuntime(t *testing.T) {
	info := Info()
	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, runtime.Version())
	}
	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Errorf("GoVersion = %q, expected to start with 'go'", info.GoVersion)
	}
}

// TestInfo_DevFallback mutates the package-level version var to force the
// fallback path. It does NOT call t.Parallel — running in parallel with
// TestInfo_VersionAlwaysSet would race on the var.
func TestInfo_DevFallback(t *testing.T) {
	origVersion := version
	t.Cleanup(func() { version = origVersion })
	version = ""

	info := Info()
	if info.Version == "" {
		t.Fatal("Version unexpectedly empty after fallback")
	}
	// Either ReadBuildInfo populated it (most environments) OR we got "dev".
	// Both are valid — the contract is "non-empty".
}
