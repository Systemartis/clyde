package claudesettings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Systemartis/clyde/internal/adapters/claudesettings"
)

// FuzzReadSettings writes the fuzzed bytes into a temp settings.json and
// asks Reader.Read() to parse them. Contract: never panic, never error
// out (Read returns an empty Settings on any failure).
func FuzzReadSettings(f *testing.F) {
	f.Add([]byte(`{"enabledPlugins":{"go":true}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"enabledPlugins":{"go":"yes"}}`))
	f.Add([]byte(`{"enabledPlugins":[]}`))
	f.Add([]byte(`not json`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		_ = claudesettings.NewAt(path).Read()
	})
}
