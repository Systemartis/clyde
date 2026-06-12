package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Systemartis/clyde/internal/adapters/anthropicapi"
	"github.com/Systemartis/clyde/internal/adapters/claudesettings"
	"github.com/Systemartis/clyde/internal/adapters/fsexplorer"
	gitadapter "github.com/Systemartis/clyde/internal/adapters/git"
	"github.com/Systemartis/clyde/internal/adapters/jsonl"
	"github.com/Systemartis/clyde/internal/adapters/lspscan"
	"github.com/Systemartis/clyde/internal/adapters/mcpconfig"
	"github.com/Systemartis/clyde/internal/adapters/processscan"
	"github.com/Systemartis/clyde/internal/adapters/systemclock"
	"github.com/Systemartis/clyde/internal/adapters/tui"
	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/project"
)

// TestComposition_LiveWiring_Constructs reproduces the composition root —
// the same adapter chain main.go assembles in live mode — against a fake
// ~/.claude/projects layout. The smoke job in CI exercises the binary
// end-to-end; this test gives us a fast, debuggable Go-level signal so a
// constructor regression doesn't first surface as a CI red on a 30s run.
//
// The test deliberately doesn't start the bubbletea program — that needs
// a TTY and is what the smoke job is for. Construction is what's been
// uncovered (PR-22 from the production-readiness analysis).
func TestComposition_LiveWiring_Constructs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	// Seed the directories the production source expects so NewProductionSource
	// returns a non-error, even if there are no actual sessions.
	if err := os.MkdirAll(filepath.Join(tmp, ".claude", "projects"), 0o700); err != nil {
		t.Fatalf("seed projects dir: %v", err)
	}
	cwd := tmp

	// === The wiring under test ===========================================

	src, err := jsonl.NewProductionSource()
	if err != nil {
		t.Fatalf("NewProductionSource: %v", err)
	}
	clk := systemclock.New()
	ls := livesession.NewWithSubagents(src, clk, src)
	ls = ls.WithGlobalSessions(src)

	gitSource := &gitadapter.Source{}
	ls = ls.WithExplorer(
		gitadapter.NewLiveSessionAdapterFor(gitSource),
		fsexplorer.NewLiveSessionAdapter(),
	)

	settings := claudesettings.New()
	ls = ls.WithMCPs(mcpconfig.NewWith(settings))
	ls = ls.WithLSPs(lspscan.NewWith(settings))
	ls = ls.WithProcesses(processscan.New())

	p := project.New(cwd)

	cfg := tui.LoadConfig()
	model := tui.NewModelLive(cfg, tui.LayoutStack, p, ls, nil, gitadapter.NewDiffAdapterFor(gitSource), src.Name())
	model = model.WithPlanUsageSource(anthropicapi.NewClient())

	// === Assertions =======================================================

	// View() must not panic on the constructed model — the same regression
	// guard the 0-width test installs, but this time on the live wire.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("model.View() panicked on live wiring: %v", r)
		}
	}()
	// We sent no WindowSizeMsg, so View() may produce an empty alt-screen.
	// Empty is acceptable; what we're guarding against here is a panic
	// or a nil-pointer dereference in the live wiring path.
	_ = model.View()

	// The source should be the live one (not a mock) — confirm the model
	// remembers the source name we threaded in.
	if got := src.Name(); got == "" {
		t.Errorf("src.Name() empty — production source should report its name")
	}
}
