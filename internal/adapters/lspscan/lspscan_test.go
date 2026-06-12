package lspscan

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Systemartis/clyde/internal/adapters/claudesettings"
)

// TestLSPs_StableSorting verifies the result is sorted alphabetically by
// display name, regardless of the order in knownLSPs.
func TestLSPs_StableSorting(t *testing.T) {
	t.Parallel()
	s := New()
	got, err := s.LSPs()
	if err != nil {
		t.Fatalf("LSPs() returned error: %v", err)
	}
	if len(got) < 2 {
		// Skip — not enough LSPs installed on this host to verify sorting.
		t.Skip("need at least 2 LSPs installed to verify sort order")
	}
	names := make([]string, len(got))
	for i, l := range got {
		names[i] = l.Name
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("LSPs() result not sorted: %v", names)
	}
}

// TestLSPs_NoDuplicates verifies that even when knownLSPs maps multiple
// binaries to the same display name (e.g. pyright + pyright-langserver), the
// result lists that name at most once.
func TestLSPs_NoDuplicates(t *testing.T) {
	t.Parallel()
	s := New()
	got, _ := s.LSPs()
	seen := map[string]bool{}
	for _, l := range got {
		if seen[l.Name] {
			t.Errorf("duplicate LSP name in result: %q", l.Name)
		}
		seen[l.Name] = true
	}
}

// TestLSPs_ActiveFlagWhenFound verifies that detected entries are marked
// Active=true. The set of detected entries depends on the host, so this
// test only asserts the property when at least one was found.
func TestLSPs_ActiveFlagWhenFound(t *testing.T) {
	t.Parallel()
	s := New()
	got, _ := s.LSPs()
	for _, l := range got {
		if !l.Active {
			t.Errorf("LSP %q has Active=false; detection implies installed", l.Name)
		}
		if l.Binary == "" {
			t.Errorf("LSP %q has empty Binary; expected resolved path", l.Name)
		}
	}
}

// TestReadClaudeEnabledLSPs_ParsesPluginKeys verifies that LSP-flavored
// plugin keys ("<name>-lsp@<source>": true) get translated to bare binary
// names. Non-LSP plugins and disabled entries must NOT appear in the
// result. This is the cross-reference data that drives the amber-dot UX:
// if an LSP is on PATH but missing from this map, the TUI flags it.
func TestReadClaudeEnabledLSPs_ParsesPluginKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	json := `{
		"enabledPlugins": {
			"gopls-lsp@claude-plugins-official": true,
			"rust-analyzer-lsp@claude-plugins-official": true,
			"engram@engram": true,
			"superpowers@claude-plugins-official": true,
			"pylsp-lsp@claude-plugins-official": false
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(json), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	s := NewWith(claudesettings.NewAt(settingsPath))
	got := s.readClaudeEnabledLSPs()

	if !got["gopls"] {
		t.Errorf("gopls should be enabled, got %v", got)
	}
	if !got["rust-analyzer"] {
		t.Errorf("rust-analyzer should be enabled, got %v", got)
	}
	if got["engram"] {
		t.Errorf("engram is NOT an LSP plugin and must not appear, got %v", got)
	}
	if got["superpowers"] {
		t.Errorf("superpowers is NOT an LSP plugin and must not appear, got %v", got)
	}
	if got["pylsp"] {
		t.Errorf("pylsp was disabled (false) and must not appear, got %v", got)
	}
}

// TestLSPs_BinaryScanCachedForLifetime is the perf-audit fix: the PATH
// scan is the expensive part (~40 exec.LookPath calls) and the snapshot
// loop calls LSPs() at 1Hz. The set of installed LSPs does not change
// during a session, so the binary scan must run exactly once, with
// subsequent calls reusing the cached scannedLSP slice.
//
// Verifies: (1) repeated LSPs() calls trigger lookPath once per binary,
// (2) the result remains correct across calls.
func TestLSPs_BinaryScanCachedForLifetime(t *testing.T) {
	t.Parallel()

	lookups := map[string]int{}
	s := NewWith(nil)
	s.lookPath = func(name string) (string, error) {
		lookups[name]++
		if name == "gopls" {
			return "/usr/local/bin/gopls", nil
		}
		return "", os.ErrNotExist
	}

	first, err := s.LSPs()
	if err != nil {
		t.Fatalf("first LSPs(): %v", err)
	}
	if len(first) != 1 || first[0].Name != "gopls" {
		t.Fatalf("first LSPs() = %+v, want one gopls entry", first)
	}

	// Knock out the lookPath so a re-scan would yield zero results.
	// If caching works, second call still returns gopls.
	calls1 := lookups["gopls"]
	for i := range 5 {
		got, err := s.LSPs()
		if err != nil {
			t.Fatalf("repeat LSPs(): %v", err)
		}
		if len(got) != 1 || got[0].Name != "gopls" {
			t.Errorf("LSPs() iteration %d = %+v, want one gopls entry (cache miss?)", i, got)
		}
	}
	if lookups["gopls"] != calls1 {
		t.Errorf("gopls lookPath called %d times after cache; want %d (no re-scan allowed)", lookups["gopls"], calls1)
	}
}

// TestReadClaudeEnabledLSPs_MissingFile returns an empty map without
// erroring. This is the graceful degradation path — the TUI continues to
// run, every LSP just defaults to ClaudeEnabled=false (amber dot).
func TestReadClaudeEnabledLSPs_MissingFile(t *testing.T) {
	t.Parallel()
	s := NewWith(claudesettings.NewAt(filepath.Join(t.TempDir(), "does-not-exist.json")))
	got := s.readClaudeEnabledLSPs()
	if len(got) != 0 {
		t.Errorf("missing settings.json should yield empty map, got %v", got)
	}
}
