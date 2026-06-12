package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunSetup_PrintsSnippetAndPath verifies `clyde setup` emits, on plain
// stdout (no TUI, no stderr noise), everything a user needs to wire hook
// notifications: the resolved hook-url path for THIS machine and a
// copy-pasteable command-type PreToolUse snippet for ~/.claude/settings.json.
func TestRunSetup_PrintsSnippetAndPath(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	var out bytes.Buffer
	if code := runSetup(&out); code != 0 {
		t.Fatalf("runSetup exit = %d, want 0", code)
	}
	got := out.String()

	wantPath := filepath.Join(cache, "clyde", "hook-url")
	if !strings.Contains(got, wantPath) {
		t.Errorf("setup output must contain the resolved hook-url path %q, got:\n%s", wantPath, got)
	}
	for _, want := range []string{`"PreToolUse"`, `"type": "command"`, "hook-url", "settings.json"} {
		if !strings.Contains(got, want) {
			t.Errorf("setup output missing %q, got:\n%s", want, got)
		}
	}
}

// TestHookURLFilePath_HonorsXDG verifies the shared path resolution used by
// both the hook server startup and `clyde setup`.
func TestHookURLFilePath_HonorsXDG(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	got, err := hookURLFilePath()
	if err != nil {
		t.Fatalf("hookURLFilePath: %v", err)
	}
	if want := filepath.Join(cache, "clyde", "hook-url"); got != want {
		t.Errorf("hookURLFilePath = %q, want %q", got, want)
	}
}
