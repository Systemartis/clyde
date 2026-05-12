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

// TestInfo_LdflagsPathReturnsVerbatim covers the goreleaser path: when all
// three vars are populated by -ldflags, Info() must return them unmodified
// without consulting debug.ReadBuildInfo (which would otherwise overwrite
// the values with the test harness's own VCS metadata).
func TestInfo_LdflagsPathReturnsVerbatim(t *testing.T) {
	origV, origC, origD := version, commit, date
	t.Cleanup(func() { version, commit, date = origV, origC, origD })

	version = "v9.9.9"
	commit = "deadbeef"
	date = "2026-01-01T00:00:00Z"

	info := Info()
	if info.Version != "v9.9.9" {
		t.Errorf("Version = %q, want v9.9.9", info.Version)
	}
	if info.Commit != "deadbeef" {
		t.Errorf("Commit = %q, want deadbeef", info.Commit)
	}
	if info.Date != "2026-01-01T00:00:00Z" {
		t.Errorf("Date = %q, want 2026-01-01T00:00:00Z", info.Date)
	}
}

// TestInfo_DevelVersionIgnored covers a subtle ReadBuildInfo edge case:
// `go build` of a module-aware binary reports Main.Version = "(devel)",
// which is not a real version. Info() must treat it the same as empty
// and fall through to the "dev" terminal fallback.
func TestInfo_DevelVersionIgnored(t *testing.T) {
	origV := version
	t.Cleanup(func() { version = origV })
	version = ""

	info := Info()
	if info.Version == "(devel)" {
		t.Errorf("Version must not be %q — Info should treat it as empty", "(devel)")
	}
	if info.Version == "" {
		t.Fatal("Version unexpectedly empty")
	}
}
