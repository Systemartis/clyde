# Clyde — UI/UX Design

> Status: **SHIPPED V1**. Locked decisions below implemented and promoted to the primary `clyde` binary.

## Locked decisions

1. **Leader key**: `ctrl+space` (cleaner than `ctrl+x`, vim-friendly).
2. **Theme**: purple-forward — Primary violet, Secondary pink, Tertiary emerald. Charm-aligned aesthetic.
3. **No "sidebar" framing**: every panel is a first-class peer in a configurable grid. The conversation panel is allocated more space because it has the most content, NOT because it's master to sidebars.
4. **Tool-results are SHOWN** — collapsed by default as `↳ tool_result …` rows; expandable on focus.
5. **Implementation approach**: **prototype-first** — mock data in one branch, build the full visual layout, iterate on the LOOK, then backfill real adapters.

## Positioning

Clyde is **Claude Code's per-project control center** — a TUI that sits in a tiled terminal pane (cmux/tmux/Ghostty) alongside `claude`, `npm run dev`, `git`. It surfaces *context around* Claude Code: sessions, tokens, todos, sub-agents, file changes, compaction warnings.

This is different from Crush (which IS Claude's chat UI) and opencode (chat-centric agent). Clyde is **multi-pane**, **read-only-first**, and **dense** — closer in spirit to k9s or lazygit than to a chat client.

## Design pillars

1. **Coherent at every terminal size.** 80×24 → 200×60+, the layout reflows; panels never break.
2. **Beautiful but informative.** Crush-level polish (semantic palette, focused/blurred states, Unicode iconography, gradients on headers); k9s-level density.
3. **Customizable.** Theme + keybindings + panel order + visibility — all via a single TOML file at `~/.config/clyde/config.toml`. Per-project override at `.clyde/config.toml`.
4. **Keyboard-first.** Leader-key model (default `ctrl+x` like opencode), vim-friendly nav, `?` for help, `:` for command palette (V2).
5. **Read-only by default.** V1 reads JSONL/git; V2+ adds prompt input and slash commands. We don't compete with `claude` — we augment it.

## Layout strategy — three breakpoints

Panels live in a **configurable grid** — no "sidebar" hierarchy, every panel is a peer. Width-driven breakpoints reduce column count and collapse less-important panels into modal overlays. Panel order and visibility are user-configurable via TOML (see Customizability).

### Breakpoint table

| Breakpoint | Width | Columns | Visible by default |
|------------|-------|---------|--------------------|
| Narrow | < 100 | 1 (stacked) | Conversation only; sidebars are modal overlays |
| Medium | 100–160 | 2 | Left sidebar + Conversation |
| Wide | 160–220 | 3 | Left sidebar + Conversation + Right sidebar |
| Ultra-wide | > 220 | 3 with extra panes | Left + Conversation + Right + extra in right column |

Height matters too — the conversation viewport scales vertically; small panels (compaction bar, status footer) have fixed height; sidebars consume remaining vertical space.

### Narrow (80×24, single column — conversation only, others as overlays)

```
╭── clyde ─ session: c0b27665 ──────────────── ▰▰▰▱▱ 62% ──╮
│                                                          │
│  10:45  user       │ ok continue                         │
│                                                          │
│  10:45  assistant  │ Hai — dispatching sdd-explore for   │
│                    │ event-rendering content...          │
│                                                          │
│  10:46  assistant  │ Tool: Read /clyde/internal/main.go  │
│  10:46  ↳ result   │ 217 lines · 6.4kb                   │
│                                                          │
│  10:47  user       │ now this is the output: …           │
│                                                          │
├──────────────────────────────────────────────────────────┤
│  ⏵ idle    in 1.2k  out 8.4k  cache 42k  $0.12          │  ← Status bar
├──────────────────────────────────────────────────────────┤
│  ?:help  ⌃␣s:sessions  ⌃␣t:todos  ⌃␣f:files  ⌃␣:leader │  ← Help footer
╰──────────────────────────────────────────────────────────╯

Sessions / Todos / Files / Subagents → modal overlays (Compositor)
e.g. press `ctrl+space` then `s` →
                ╔═ Sessions ════════════════════╗
                ║ ●  c0b27665  main      12.3k  ║
                ║    160196cc  refactor   8.1k  ║
                ║    ab9f4422  parser     2.4k  ║
                ║                               ║
                ║ ↵ select   esc cancel         ║
                ╚═══════════════════════════════╝
```

### Medium (140×40, two columns — conversation paired with stacked context panels)

```
╭── Sessions ──────────╮╭── session: main ─────────────────── ▰▰▰▱▱ 62% ──╮
│ ●  main      12.3k   ││                                                  │
│    refactor   8.1k   ││  10:45  user       │ ok continue                 │
│    parser     2.4k   ││                                                  │
╰──────────────────────╯│  10:45  assistant  │ Hai — dispatching           │
╭── Todos ─────────────╮│                    │ sdd-explore for             │
│ ☐ add tests          ││                    │ event-rendering content…    │
│ ✓ fix parser bug     ││                                                  │
│ ☐ wire up TUI        ││  10:46  assistant  │ Tool: Read /path/main.go    │
╰──────────────────────╯│  10:46  ↳ result   │ 217 lines · 6.4kb           │
╭── Recent files ──────╮│                                                  │
│ main.go      2m  +3  ││  10:47  user       │ now this is the output: …   │
│ jsonl.go     5m  +1  ││                                                  │
│ format.go    8m  ✦   ││                    ⠋ Claude is responding…       │
│ event.go    14m  +5  │╰──────────────────────────────────────────────────╯
╰──────────────────────╯╭──────────────────────────────────────────────────╮
                        │  ⏵ responding   in 1.2k  out 8.4k  $0.12  ●main │
╭───────────────────────┴──────────────────────────────────────────────────╮
│  ?:help   ⌃␣s:sessions   ⌃␣t:todos   ⌃␣f:files   ⌃␣a:subagents   ⌃␣ldr  │
╰──────────────────────────────────────────────────────────────────────────╯
```

### Wide (180×50, three-column)

```
┌── Sessions ──────────┬── session: main ─ ▰▰▰▱▱ 62% ─────────────┬── Subagents ──────────┐
│ ●  main      12.3k   │                                          │ ▶  explore-content    │
│    refactor   8.1k   │  10:45  user       │ ok continue          │    ├ Read jsonl.go    │
│    parser     2.4k   │                                          │    ├ Read events_*    │
│                      │  10:45  assistant  │ Hai — dispatching    │    └ Read spec.md     │
├──────────────────────┤                    │ sdd-explore for      │ ✓  propose-rendering  │
│ Todos                │                    │ event-rendering...   │ ✓  spec-rendering     │
│ ☐ add tests          │                                          │ ◉  apply-batch-A      │
│ ✓ fix parser bug     │  10:46  assistant  │ Tool: Read /path...  │ ──────────────────    │
│ ☐ wire up TUI        │                                          │ Compaction            │
│                      │  10:47  user       │ now this is the      │ ▰▰▰▰▰▱▱▱▱▱  62%       │
├──────────────────────┤                    │ output: ...          │  in   1.2k            │
│ Recent files         │                                          │  out  8.4k            │
│ main.go    2m  +3    │  10:48  assistant  │ Tool: Edit jsonl.go  │  cache 42k            │
│ jsonl.go   5m  +1    │                                          │  $    0.12            │
│ format.go  8m  ✦     │                    ⠋ Claude is responding │ ──────────────────    │
│ event.go  14m  +5    │                                          │ Files                 │
├──────────────────────┤                                          │ ▼ internal/           │
│ Status                                                          │   ▼ adapters/         │
│ git: main · clean    │                                          │     ▶ jsonl/          │
│ go: 1.26.2           │                                          │     ▶ tui/            │
│ tests: 73 ✓          │                                          │   ▼ domain/           │
└──────────────────────┴──────────────────────────────────────────┴───────────────────────┘
│  ?:help  s:sessions  t:todos  f:files  a:subagents  c:compact  /:cmd  ⌃x:leader         │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

## Panel inventory

| # | Panel | Default column (wide) | Widget | Content rules |
|---|-------|----------------------|--------|---------------|
| 1 | **Conversation** | Center (largest flex) | `bubbles/v2/viewport` + `glamour` | Filtered events (no opaques/meta/tool-result-only-user); user prompts as plain text; assistant text via glamour markdown; tool_use as `Tool: <Name> <key-arg>`; **tool_result as collapsed `↳ result` line by default** (expandable on focus + Enter); thinking blocks as ⠋ icon row |
| 2 | **Sessions** | Left, top | `bubbles/v2/list` | Per-project sessions sorted by LastActivity desc; ● for focused; token total per session; `↵` to switch focus |
| 3 | **Todos** | Left, middle | Custom delegate over `list` | ☐/✓ icons; cwd-scoped; live update from `~/.claude/todos/<sid>-*.json` |
| 4 | **Recent files** | Left, bottom | `bubbles/v2/table` | Last N files Claude touched; columns: name, age, delta (`+lines`/`✦` for new); `↵` opens in `$EDITOR` |
| 5 | **Subagent tree** | Right, top (wide only) | `bubbles/v2/table` w/ indent | Tree of running/recent sub-agents; ▶ running, ✓ done, ✗ failed; nested calls indented |
| 6 | **Compaction** | Right, middle | `bubbles/v2/progress` | Token-cap proximity vs model context limit; ▰▰▰▱▱ + percent; warns at 75%, errors at 90% |
| 7 | **File explorer** | Right, bottom (wide only) | `bubbles/v2/filepicker` | Project tree; expand/collapse; image preview pane (V2 — gated on cmux+Kitty validation) |
| 8 | **Status bar** | Header band + footer band | Custom lipgloss render | Header: session ID + compaction bar; footer: state (idle/responding/tool-calling), running totals, cost, git branch+dirty status |

> **Note on layout language**: every panel is a peer in a configurable grid. The "Center" / "Left" / "Right" labels above describe the DEFAULT position only — users freely re-arrange via `[layout].columns_at_*` config arrays.

## Theme system

Adopt Crush's `charmtone` pattern — a `Styles` struct with semantic roles, populated at startup from a theme spec.

### Semantic palette (default — `clyde-dark`)

```go
type Palette struct {
    Primary       lipgloss.Color  // session names, focused borders, gradient endpoints
    Secondary     lipgloss.Color  // sessions list highlight, active tool calls
    Tertiary      lipgloss.Color  // todos checkmarks, success states

    BgBase        lipgloss.Color  // outer background
    BgSubtle      lipgloss.Color  // panel-differentiation background
    BgAccent      lipgloss.Color  // selected row, focused panel inner

    FgBase        lipgloss.Color  // primary text
    FgMuted       lipgloss.Color  // labels, footer text
    FgSubtle      lipgloss.Color  // dimmed metadata (timestamps)

    BorderColor       lipgloss.Color  // blurred panel border
    BorderColorFocus  lipgloss.Color  // focused panel border (Primary)

    Warning  lipgloss.Color  // compaction at 75%+
    Error    lipgloss.Color  // compaction at 90%+, failures
    Success  lipgloss.Color  // ✓ marks, completed todos
    Info     lipgloss.Color  // hint badges
}
```

### Default values (`clyde-dark` — purple-forward)

| Role | Hex | Why |
|------|-----|-----|
| Primary | `#A78BFA` (lavender violet) | Modern Charm-aligned purple, vibrant but readable, evokes the "magical companion" identity |
| Secondary | `#F472B6` (soft pink) | Charm signature pairing — purple+pink gradient endpoints, harmonious not competing |
| Tertiary | `#34D399` (emerald) | Success/completion — distinct hue family from accent |
| BgBase | `#0F0E17` | Near-black with violet undertone — lets purple pop without grayscale flatness |
| BgSubtle | `#1E1B2E` | Panel backgrounds — slight violet tint differentiates from base |
| BgAccent | `#2D2A3E` | Selected rows — readable contrast against BgSubtle |
| FgBase | `#E0DEF4` | Off-white with cool tint — sympathetic with purple chrome |
| FgMuted | `#9892B5` | Cool gray for labels |
| FgSubtle | `#5A5577` | Dimmed metadata (timestamps, paths) |
| BorderColor | `#312E45` | Quiet borders for blurred panels — clearly subordinate |
| BorderColorFocus | `#A78BFA` (Primary) | Active panel pops with violet glow |
| Warning | `#FBBF24` (amber) | Warm warning, distinct from any chrome color |
| Error | `#F87171` (coral) | Soft red — not jarring against purple palette |
| Success | `#34D399` (emerald) | Same as Tertiary |
| Info | `#60A5FA` (sky blue) | Cool blue for hint/info badges |

### Header gradient

The session-name header uses Crush's `ApplyBoldForegroundGrad()` for a gradient from `Primary` (violet) → `Secondary` (pink). Subtle, not garish — applied only to the focused-session label, ~12 chars wide. This is the one signature flourish per screen.

### Built-in themes (V1)

- `clyde-dark` (default)
- `clyde-light` (light terminals; brighter Primary, dark FgBase)
- `catppuccin-mocha`, `catppuccin-latte` (community-loved schemes)
- `tokyo-night`
- `gruvbox-dark`

User picks via `theme = "clyde-dark"` in config. V2: full custom theme files at `~/.config/clyde/themes/<name>.toml`.

### Markdown rendering

Glamour is used inside the conversation viewport for assistant text content. Style is bound to the active theme (theme TOML carries a `glamour_style` key pointing at one of glamour's built-in styles or a custom JSON stylesheet path).

### Focused vs blurred panel states

- **Focused**: `BorderColorFocus` (Primary) on the panel border + slightly brighter background tint.
- **Blurred**: `BorderColor` muted gray + subtle background.
- Tab cycles focus; mouse click on a panel focuses it (lipgloss-mouse zone detection in v2).

### Iconography

Unicode-only. NEVER emoji (consistency, no font dependency, terminal-portable).

| Icon | Use |
|------|-----|
| `●` | Active/focused session |
| `○` | Inactive session |
| `✓` | Completed |
| `☐` | Todo unchecked |
| `✗` | Failed/error |
| `⏵` | Idle indicator |
| `⠋⠙⠹⠸` | Spinner (dots style) |
| `▶` | Running subagent / expandable tree node |
| `▼` | Expanded tree node |
| `▰▰▰▱▱` | Progress bar fill |
| `┃` | Scrollbar |
| `│` | Vertical separator |
| `─` `═` | Horizontal separator (single = blurred, double = focused) |
| `✦` | New file |
| `+N`/`-N` | Diff hunks |

## Keyboard model

### Top-level (always active)

| Key | Action |
|-----|--------|
| `?` | Toggle help overlay (auto-built from key bindings) |
| `q` / `ctrl+c` | Quit |
| `tab` | Cycle focus to next panel |
| `shift+tab` | Cycle focus to previous panel |
| `ctrl+space` | **Leader** — wait for chord |
| `:` | Command palette (V2) |
| `/` | Search current panel (V2) |

### Leader chords (`ctrl+space` then…)

Modeled on opencode's leader pattern but with `ctrl+space` for cleaner ergonomics. Single chord per action; chord overlay shows on hold.

| Chord | Action |
|-------|--------|
| `s` | Sessions list (overlay or focus pane) |
| `n` | New focus on most-recent session |
| `t` | Todos panel/overlay |
| `f` | File explorer (V2) |
| `a` | Subagent tree |
| `c` | Compaction details |
| `r` | Reload (re-read JSONL from disk) |
| `T` | Theme switcher (cycle built-in themes) |
| `?` | Show all leader chords |

### Panel-specific (when focused)

- **Conversation**: `j`/`k` or `↓`/`↑` to scroll line; `ctrl+d`/`ctrl+u` half-page; `g`/`G` top/bottom; `space` page down.
- **Sessions list**: `j`/`k` to navigate; `↵` to focus that session in conversation pane; `d` to delete session JSONL (with confirm; V2).
- **Todos**: `j`/`k`; `space` to mark done (V2 — would write to todo file).
- **Files**: `j`/`k`; `↵` to open in `$EDITOR`.
- **Subagents**: `j`/`k`; `↵` to expand/show details overlay.

### Configurable

All bindings exposed as `[keybinds]` section in TOML. Defaults shipped; user overrides win.

```toml
[keybinds]
quit = ["q", "ctrl+c"]
leader = "ctrl+space"
help = "?"
focus_next = "tab"
focus_prev = "shift+tab"

[keybinds.leader]
sessions = "s"
todos    = "t"
files    = "f"
# ...
```

## Customizability

### Config — `~/.config/clyde/config.toml` (XDG-compliant)

```toml
# Theme
theme = "clyde-dark"  # or "clyde-light", "catppuccin-mocha", etc.

# Layout
[layout]
narrow_width = 100
medium_width = 160
wide_width   = 220

# Panels by column at each breakpoint. Conversation is one panel among peers,
# allocated more flex weight because it has the most content.
columns_at_wide   = [["sessions", "todos", "recent_files", "status"], ["conversation"], ["subagents", "compaction", "files"]]
columns_at_medium = [["sessions", "todos", "recent_files"], ["conversation"]]
columns_at_narrow = [["conversation"]]   # everything else is a modal overlay

# Conversation panel flex weight relative to peer columns (1.0 = equal).
conversation_flex = 2.5

[panels.sessions]
enabled = true
show_token_total = true
sort = "last_activity"  # or "name", "created"

[panels.todos]
enabled = true
show_completed = false  # default hide checked

[panels.recent_files]
enabled = true
limit = 10
relative_paths = true

[panels.subagents]
enabled = true
flatten_depth = 2

[panels.compaction]
warn_threshold = 0.75
error_threshold = 0.90

# Conversation behavior
[conversation]
follow_mode = true             # auto-scroll on new events
glamour_style = "auto"         # "dark", "light", "auto", path to custom JSON
summary_max_chars = 80
hide_thinking = false          # show "(thinking)" rows or skip entirely
tool_results = "collapsed"     # "collapsed" (default — ↳ result lines), "expanded", or "hidden"

# Status bar
[status]
show_cost = true
show_git = true
show_test_count = false  # opt-in; requires `go test -count` invocation
```

### Per-project override — `<project>/.clyde/config.toml`

Same schema; merged on top of user config. Useful for project-specific themes or panel toggles.

### Themes — `~/.config/clyde/themes/<name>.toml`

Full palette override. Just key/value pairs matching the `Palette` struct.

```toml
# ~/.config/clyde/themes/sunset.toml
primary    = "#FF6B35"
secondary  = "#F7C59F"
tertiary   = "#EFEFD0"
bg_base    = "#1A1423"
# ... etc
```

## Visual polish details

1. **Borders**: rounded (`╭╮╰╯`) for panel borders, single-line (`─│`) for inner separators. Thicker (`━┃`) for the focused panel border *only* if the focus state needs an extra signal (test it both ways at impl).
2. **Padding**: 1 space horizontal inside panels; 0 vertical (lines are dense).
3. **Scrollbars**: `┃` track on the right edge of viewport panels; auto-hide if content fits.
4. **Truncation**: `…` (U+2026) per existing event.Truncate. Filenames truncate from middle (`/very/long/path/to/file.go` → `/very/.../file.go`).
5. **Loading states**: spinner next to "responding..." text in conversation header.
6. **Empty states**: every panel has a friendly empty-state line — "No sessions yet.", "No todos.", "No files touched.", "No subagents running."
7. **Focused header**: gradient on session name (Primary → Secondary, ~12 chars). One subtle flourish per screen — don't over-do.

## What clyde does NOT do (out of scope, prevents creep)

- Does not have a chat input by default (V2 may add — but Claude has its own input). Read-only-first.
- Does not show MCP server status (separate panel idea, V3+).
- Does not modify session JSONLs or todos (read-only).
- Does not embed `claude` itself. It runs ALONGSIDE.
- Does not show diff views inline yet (V2 — opens `$EDITOR` or `delta` on demand).
- Does not show cost graphs over time (V2+ — needs aggregation across sessions).

## Implementation approach: prototype-first (LOCKED)

We build the visual prototype with **mock data** first — fake sessions, fake events, fake todos, fake subagents, fake files. The prototype proves the layout, theme, focus model, and breakpoints work end-to-end BEFORE wiring real adapters.

**Prototype scope (one branch, one push)**:

1. New `internal/adapters/tui/` rewrite with the full layout (all 8 panels at all 3 breakpoints).
2. Theme system (`Palette` struct, `Styles` struct, `clyde-dark` purple default).
3. Layout engine (breakpoint detector, `JoinHorizontal`/`JoinVertical` composer, `Compositor` for narrow-mode overlays).
4. Focus state machine (Tab cycles, focused/blurred borders, Compositor-aware modal focus capture).
5. Mock data fixtures (`internal/adapters/tui/mockdata/`) with realistic content for every panel.
6. Glamour wired into the conversation viewport for assistant text.
7. Help overlay built from `key.Binding` registry.
8. Empty-state lines per panel.

**Out of scope for prototype** (deferred to "wire-up" phase):
- Real session JSONL data (use mocks).
- Config file loading (hardcode `clyde-dark`, default keybinds).
- Per-project config merge.
- File explorer + image preview (V2 entirely).
- Subagent expand-on-Enter modal details (just static tree).

**Iteration loop during prototype**:
- Run `clyde-proto` (separate binary, side-by-side with `clyde`) inside cmux.
- Resize the pane → see layout reflow.
- Tab through panels → see focus indicators.
- Adjust palette / spacing / typography until it looks right.

**Wire-up phase (after prototype is signed off)**:

The broad order of work to swap mocks for real data:

1. **Theme system** — `Palette` struct, default `clyde-dark`, theme loader from TOML, `Styles` struct that consumes the palette. (Small, foundational.)
2. **Layout primitives** — width-based breakpoint detector; column composer using `JoinHorizontal`/`JoinVertical`; focused/blurred state machine.
3. **Conversation pane redesign** — viewport-based, glamour for assistant markdown, status header band with gradient + compaction bar.
4. **Sessions panel** — list bubble; focus-switch wiring with the use case.
5. **Todos panel** — adapter for `~/.claude/todos/`; list with custom delegate.
6. **Recent files panel** — extract from existing tool_use events (Edit/Write/MultiEdit); table.
7. **Subagent panel** — queue-operation modeling; tree render.
8. **Compaction bar** — model context limit lookup table; progress bubble.
9. **Status bar** — git branch reader, cost summer, idle/responding state machine.
10. **Keybind system** — central registry; help bubble; configurable.
11. **Config loader** — TOML + per-project merge.
12. **File explorer** (V2) — filepicker; image preview gated on Kitty graphics.

Each post-prototype step is a candidate SDD change once the visual prototype is approved.
