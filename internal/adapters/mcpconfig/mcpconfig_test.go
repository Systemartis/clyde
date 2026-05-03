package mcpconfig_test

import (
	"path/filepath"
	"testing"

	"github.com/clyde-tui/clyde/internal/adapters/claudesettings"
	"github.com/clyde-tui/clyde/internal/adapters/mcpconfig"
)

func TestSource_MCPs_fixture(t *testing.T) {
	src := mcpconfig.NewWith(claudesettings.NewAt(filepath.Join("testdata", "settings.json")))

	mcps, err := src.MCPs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fixture has 4 plugins, sorted alphabetically.
	if len(mcps) != 4 {
		t.Fatalf("expected 4 MCPs, got %d", len(mcps))
	}

	// Verify sorted order and enabled flags.
	want := []struct {
		name    string
		enabled bool
	}{
		{"claude-code-setup@claude-plugins-official", true},
		{"engram@engram", true},
		{"frontend-design@claude-plugins-official", false},
		{"superpowers@claude-plugins-official", true},
	}

	for i, w := range want {
		got := mcps[i]
		if got.Name != w.name {
			t.Errorf("[%d] name: want %q, got %q", i, w.name, got.Name)
		}
		if got.Enabled != w.enabled {
			t.Errorf("[%d] enabled: want %v, got %v", i, w.enabled, got.Enabled)
		}
		if got.ToolCount != 0 {
			t.Errorf("[%d] tool count: want 0, got %d", i, got.ToolCount)
		}
	}
}

func TestSource_MCPs_missing_file(t *testing.T) {
	src := mcpconfig.NewWith(claudesettings.NewAt(filepath.Join("testdata", "nonexistent.json")))

	mcps, err := src.MCPs()
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if len(mcps) != 0 {
		t.Errorf("expected empty slice for missing file, got %d entries", len(mcps))
	}
}

func TestSource_MCPs_empty_plugins(t *testing.T) {
	src := mcpconfig.NewWith(claudesettings.NewAt(filepath.Join("testdata", "settings_empty.json")))

	mcps, err := src.MCPs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mcps) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(mcps))
	}
}

func TestSource_MCPs_malformed(t *testing.T) {
	src := mcpconfig.NewWith(claudesettings.NewAt(filepath.Join("testdata", "settings_malformed.json")))

	mcps, err := src.MCPs()
	if err != nil {
		t.Fatalf("expected nil error for malformed JSON, got: %v", err)
	}
	if len(mcps) != 0 {
		t.Errorf("expected empty slice for malformed JSON, got %d entries", len(mcps))
	}
}

// TestSource_satisfies_ports_MCPSource verifies at compile time that mcpconfig.Source
// implements ports.MCPSource. The blank-identifier assignment in mcpconfig.go
// enforces this; this test documents the contract.
func TestSource_satisfies_ports_MCPSource(t *testing.T) {
	// If mcpconfig.Source stops satisfying ports.MCPSource, the package-level
	// var _ ports.MCPSource = Source{} in mcpconfig.go will fail to compile —
	// this test doesn't need to do anything extra.
	t.Log("mcpconfig.Source satisfies ports.MCPSource (enforced at package level)")
}
