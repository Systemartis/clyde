package clydelog

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetup_HonorsCLYDELogFile verifies CLYDE_LOG_FILE wins over the XDG
// fallback. A test binary that mutates user $HOME or $XDG_CACHE_HOME is
// brittle; the env-var override exists precisely so tests can pin the path.
func TestSetup_HonorsCLYDELogFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	t.Setenv("CLYDE_LOG_FILE", path)
	t.Setenv("CLYDE_DEBUG", "")

	resolved, closer, err := Setup()
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer closer.Close()

	if resolved != path {
		t.Errorf("path = %q, want %q", resolved, path)
	}

	slog.Info("hello", slog.String("k", "v"))

	// Force the underlying file to flush by closing it now and re-reading.
	if err := closer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(raw), `"msg":"hello"`) {
		t.Errorf("log file missing %q record:\n%s", "hello", raw)
	}

	// Each line is a JSON record; sanity-check parse.
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("line is not valid JSON: %q (%v)", line, err)
		}
	}
}

// TestSetup_DebugEnvRaisesLevel verifies CLYDE_DEBUG flips the threshold to
// Debug. Without it, slog.Debug records are filtered out.
func TestSetup_DebugEnvRaisesLevel(t *testing.T) {
	for _, tc := range []struct {
		name      string
		debug     string
		wantDebug bool
	}{
		{"unset", "", false},
		{"set-to-1", "1", true},
		{"set-to-anything", "yes", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.log")
			t.Setenv("CLYDE_LOG_FILE", path)
			t.Setenv("CLYDE_DEBUG", tc.debug)

			_, closer, err := Setup()
			if err != nil {
				t.Fatalf("Setup: %v", err)
			}
			defer closer.Close()

			slog.Debug("debug-record")
			_ = closer.Close()

			raw, _ := os.ReadFile(path)
			gotDebug := strings.Contains(string(raw), `"msg":"debug-record"`)
			if gotDebug != tc.wantDebug {
				t.Errorf("CLYDE_DEBUG=%q -> debug emitted=%v, want=%v\nfile=%s", tc.debug, gotDebug, tc.wantDebug, raw)
			}
		})
	}
}

// TestSetup_FallsBackToDiscard verifies a path that can't be created produces
// a working logger (no panic) but no file. Useful when $HOME is read-only
// in containers / CI.
func TestSetup_FallsBackToDiscard(t *testing.T) {
	// Point the path inside a regular file — MkdirAll will fail.
	notADir := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Setenv("CLYDE_LOG_FILE", filepath.Join(notADir, "clyde.log"))

	path, closer, err := Setup()
	if err == nil {
		t.Fatal("expected an error when log dir cannot be created")
	}
	defer closer.Close()
	if path != "" {
		t.Errorf("path = %q, want empty on fallback", path)
	}

	// The default logger must still be usable — no panic.
	slog.Info("after-fallback")
}
