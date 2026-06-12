package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteHookURLFile_ModeAndContent verifies the two security properties
// of the hook-url file: it carries the token-bearing URL (plus trailing
// newline for clean $(cat ...) interpolation) and is mode 0600 so other
// local users can't read the auth token.
func TestWriteHookURLFile_ModeAndContent(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	const url = "http://127.0.0.1:49217/hook?t=secret-token"
	path, err := writeHookURLFile(url)
	if err != nil {
		t.Fatalf("writeHookURLFile: %v", err)
	}
	if want := filepath.Join(cache, "clyde", "hook-url"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 600 — the URL embeds the auth token", got)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != url+"\n" {
		t.Errorf("content = %q, want %q", body, url+"\n")
	}
}

// TestWriteHookURLFile_FallsBackToHomeCache verifies the ~/.cache fallback
// when XDG_CACHE_HOME is unset.
func TestWriteHookURLFile_FallsBackToHomeCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")

	path, err := writeHookURLFile("http://127.0.0.1:1/hook?t=x")
	if err != nil {
		t.Fatalf("writeHookURLFile: %v", err)
	}
	if want := filepath.Join(home, ".cache", "clyde", "hook-url"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

// TestRunCrashReport_ExitZero smoke-tests the user-facing wrapper against a
// temp HOME: it must succeed and leave the tarball where it promised.
func TestRunCrashReport_ExitZero(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	if code := runCrashReport(); code != 0 {
		t.Fatalf("runCrashReport = %d, want 0", code)
	}
	matches, err := filepath.Glob(filepath.Join(home, "clyde-crash-*.tar.gz"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected exactly one crash tarball in $HOME, got %v (err %v)", matches, err)
	}
}
