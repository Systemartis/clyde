// Command clyde is the composition root for the clyde TUI.
// By default it reads real Claude Code session data from
// ~/.claude/projects/<encoded-cwd>/.
//
// Pass --demo to run on deterministic mock data (good for golden tests and
// offline development).
//
// Usage:
//
//	clyde [--layout=stack|tabs|multi-col] [--demo]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Systemartis/clyde/internal/adapters/anthropicapi"
	"github.com/Systemartis/clyde/internal/adapters/claudesettings"
	"github.com/Systemartis/clyde/internal/adapters/fsexplorer"
	gitadapter "github.com/Systemartis/clyde/internal/adapters/git"
	"github.com/Systemartis/clyde/internal/adapters/hookserver"
	"github.com/Systemartis/clyde/internal/adapters/jsonl"
	"github.com/Systemartis/clyde/internal/adapters/lspscan"
	"github.com/Systemartis/clyde/internal/adapters/mcpconfig"
	"github.com/Systemartis/clyde/internal/adapters/processscan"
	"github.com/Systemartis/clyde/internal/adapters/systemclock"
	"github.com/Systemartis/clyde/internal/adapters/tui"
	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/project"
)

func main() {
	os.Exit(run())
}

func run() int {
	var layoutFlag string
	var demoFlag bool
	var sourceFlag string

	flag.StringVar(&layoutFlag, "layout", "", "layout mode override: stack, tabs, multi-col")
	flag.BoolVar(&demoFlag, "demo", false, "use mock data instead of live Claude Code data")
	flag.StringVar(&sourceFlag, "source", "claude", "LLM source adapter: claude (default; gemini/codex/kimi are V21+)")
	flag.Parse()

	// Validate source flag — only "claude" is implemented; warn on others.
	switch sourceFlag {
	case "claude", "claude-code":
		// OK — default adapter
	case "gemini", "codex", "kimi":
		fmt.Fprintf(os.Stderr, "clyde: --source=%s is not yet implemented (V21+ roadmap); falling back to claude\n", sourceFlag)
		sourceFlag = "claude"
	default:
		fmt.Fprintf(os.Stderr, "clyde: unknown source %q (valid: claude); falling back to claude\n", sourceFlag)
		sourceFlag = "claude"
	}

	cfg := tui.LoadConfig()

	mode := cfg.Layout.DefaultMode
	if layoutFlag != "" {
		switch layoutFlag {
		case "stack":
			mode = tui.LayoutStack
		case "tabs":
			mode = tui.LayoutTabs
		case "multi-col":
			mode = tui.LayoutMultiCol
		default:
			fmt.Fprintf(os.Stderr, "clyde: unknown layout %q (valid: stack, tabs, multi-col)\n", layoutFlag)
			return 1
		}
	}

	var model tui.Model

	if demoFlag {
		// Demo mode: deterministic mock data, no live reads, no hook server.
		model = tui.NewModelWithConfig(cfg, mode)
	} else {
		// Live mode: read real Claude Code sessions + start hook server.
		src, err := jsonl.NewProductionSource()
		if err != nil {
			// Cannot determine home dir — fall back to demo mode gracefully.
			fmt.Fprintf(os.Stderr, "clyde: cannot initialize production source: %v\n", err)
			fmt.Fprintln(os.Stderr, "clyde: falling back to demo mode")
			model = tui.NewModelWithConfig(cfg, mode)
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "clyde: cannot determine cwd: %v\n", err)
				fmt.Fprintln(os.Stderr, "clyde: falling back to demo mode")
				model = tui.NewModelWithConfig(cfg, mode)
			} else {
				clk := systemclock.New()
				// Per-project config overrides (Phase 4) are applied inside
				// NewModelLive, which keeps both baseCfg (raw file) and cfg
				// (effective merged) so the settings overlay can edit either.
				_ = cwd
				// Wire subagent source so Phase B subagent timelines are built.
				ls := livesession.NewWithSubagents(src, clk, src)
				// Wire global session source for cross-project 5h/7d usage aggregation.
				ls = ls.WithGlobalSessions(src)
				// Phase D: wire git + filesystem explorer adapters.
				// One git.Source is shared between the LiveSession adapter
				// (Status) and the Diff adapter (Diff) so their per-tick
				// caches coalesce — without sharing, each adapter spawns
				// its own `git status` / `git diff` subprocesses.
				gitSource := &gitadapter.Source{}
				ls = ls.WithExplorer(
					gitadapter.NewLiveSessionAdapterFor(gitSource),
					fsexplorer.NewLiveSessionAdapter(),
				)
				// Phase G: wire MCP config from ~/.claude/settings.json.
				// One settings.json Reader is shared between mcpconfig and
				// lspscan so the parse caches coalesce — without sharing,
				// the snapshot loop would re-read+parse the same file twice
				// per second.
				settings := claudesettings.New()
				ls = ls.WithMCPs(mcpconfig.NewWith(settings))
				// Detect installed LSPs by scanning $PATH for known binaries.
				ls = ls.WithLSPs(lspscan.NewWith(settings))
				// Detect running `claude --session-id <X>` processes so
				// `/resume`-d-but-idle sessions still earn a tab in the
				// title-bar strip. Cwd filtering happens by construction —
				// the per-cwd session list bounds what the probe can mark
				// live.
				ls = ls.WithProcesses(processscan.New())
				p := project.New(cwd)

				// Phase H: start the localhost hook server on a random port.
				// Lifecycle is tied to the Bubble Tea program context.
				hookCtx, hookCancel := context.WithCancel(context.Background())
				defer hookCancel()
				var hs *hookserver.Server
				hs, err = hookserver.New()
				if err != nil {
					fmt.Fprintf(os.Stderr, "clyde: hook server unavailable: %v (continuing without hooks)\n", err)
				} else {
					go func() {
						if serveErr := hs.Start(hookCtx); serveErr != nil {
							fmt.Fprintf(os.Stderr, "clyde: hook server error: %v\n", serveErr)
						}
					}()
					fmt.Fprintf(os.Stderr, "clyde: hook server on port %d (per-process auth token attached)\n", hs.Port())
					fmt.Fprintf(os.Stderr, "clyde: add to ~/.claude/settings.json:\n")
					fmt.Fprintf(os.Stderr, `  "hooks": { "PreToolUse": [{ "type": "http", "url": "%s" }] }%s`, hs.URL(), "\n")
				}

				// Phase E: wire git diff adapter — shares the gitSource cache
				// with the LiveSession adapter above.
				diffAdapter := gitadapter.NewDiffAdapterFor(gitSource)
				model = tui.NewModelLive(cfg, mode, p, ls, hs, diffAdapter, src.Name())

				// V21: wire Anthropic plan-usage source so the usage panel
				// shows the SAME percentages as claude.ai/settings/usage.
				// Refreshes every 5 min; falls back to time-elapsed
				// approximation when credentials are missing or the API is
				// unreachable.
				model = model.WithPlanUsageSource(anthropicapi.NewClient())
			}
		}
	}

	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "clyde:", err)
		return 1
	}
	return 0
}
