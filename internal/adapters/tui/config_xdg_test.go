package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfig_HonorsXDGConfigHome verifies the config path follows the
// XDG base-directory spec: when XDG_CONFIG_HOME is set, the config lives at
// $XDG_CONFIG_HOME/clyde/config.toml instead of ~/.config/clyde/config.toml.
// The cache path (hook-url, logs) already honors XDG_CACHE_HOME — the config
// must not behave differently, or sandboxed runs (tests, VHS recordings)
// silently read and WRITE the user's real config.
func TestLoadConfig_HonorsXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	dir := filepath.Join(xdg, "clyde")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("theme = \"nord\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := LoadConfig()
	if cfg.Theme != ThemeNord {
		t.Errorf("LoadConfig().Theme = %q; want %q (from $XDG_CONFIG_HOME/clyde/config.toml)", cfg.Theme, ThemeNord)
	}
}

// TestWriteConfigFile_HonorsXDGConfigHome verifies settings persistence
// writes to the XDG-resolved path, not the hardcoded home path.
func TestWriteConfigFile_HonorsXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	cfg := DefaultConfig()
	cfg.Theme = ThemeDracula
	writeConfigFile(cfg)

	data, err := os.ReadFile(filepath.Join(xdg, "clyde", "config.toml"))
	if err != nil {
		t.Fatalf("config not written under $XDG_CONFIG_HOME: %v", err)
	}
	if !containsLine(string(data), `theme = "dracula"`) {
		t.Errorf("written config missing theme; got:\n%s", data)
	}
}

func containsLine(s, want string) bool {
	for _, line := range splitLines(s) {
		if line == want {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
