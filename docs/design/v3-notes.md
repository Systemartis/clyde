# clyde — design notes & feature backlog

A TUI companion for Claude Code. Opens alongside `claude` in a split or separate window. Read-mostly observer with light interactivity (open files, view images, approve permission prompts). Aesthetic borrows from Charm tools (crush, opencode) — vibrant accents on a Tokyo Night–ish dark surface, lowercase labels riding the panel borders.

## naming

- **claude** — the AI doing the work. Author of all tool calls, edits, and permission requests.
- **clyde** — the companion app showing what claude is doing. Never speaks in first person, never claims credit for claude's actions.

Notification copy is always `claude wants to ...` not `clyde wants to ...`. clyde is the messenger.

---

## layout (current)

Three columns under a slim title bar, with a notification banner and keybind status bar below the grid.

```
┌─ clyde · ~/path · ●live · 1h24m · 47k · $1.42 ─────────────┐
├──────────┬──────────────────────────┬─────────────────────┤
│ explorer │ now (mascot + status)    │ usage               │
│          ├──────────────────────────┤                     │
│ modified ├──────────────────────────┤                     │
│ section  │ tasks (scrollable)       │ servers (mcps+lsps) │
│          │   ✓ done · 0:12          │                     │
│ tree     │   ▸ active (expanded)    │                     │
│          │   □ pending              │ images              │
│          ├──────────────────────────┤                     │
│ ↵ ⌘e /   │ live diff (scrollable)   │                     │
├──────────┴──────────────────────────┴─────────────────────┤
│ ◆ claude wants to run `npm test`     [y] [n] [esc]         │
├────────────────────────────────────────────────────────────┤
│ ⌃k palette · ⌃e explorer · ⌃t tasks · ⌃d diff · ↵ ready    │
└────────────────────────────────────────────────────────────┘
```

Column widths: 170 / 310 / flex. Scrollable panels use a thin custom scrollbar (`#3b4261` thumb on transparent track).

---

## panels — what each one is for

**explorer** — the dev's editor handles the real tree, but clyde shows two things the editor doesn't: a `modified` section pinned at top with `+/−` counts (Claude's session diff at a glance), and the currently-active file highlighted in coral. Clicking opens a read-only viewer; `⌘e` opens an edit buffer; `/` filters.

**now** — what claude is doing this second. Mascot on the left, status text on the right (`edit auth.ts`, `14 lines staged · line 46`, `● ts ● lint ▸ compile`). Spinner + throughput. Single source of truth for "is it alive."

**tasks** — the plan. Most unique value of the whole app. Pulled from claude's `TodoWrite` calls (or inferred from the conversation). Done tasks get a small subtitle line with what was learned (`3 of 18 failed in auth.test.ts`, `expired tokens crash verifyToken`) so the dev can audit reasoning, not just completion. Active task expands inline with substeps + progress bar + current operation. Pending tasks dim out.

**live diff** — accumulating diff for whatever claude is currently editing. Hunks separated by `@@` lines. Scrollable.

**usage** — tokens (with shimmer bar, gradient by % consumed), cost, turns, model. Divider, then issues counts (errors, warnings, tests). Single panel because cost and code-health are both "is this going well?" signals.

**servers** — MCPs and LSPs in one panel under sub-headers. Status dot pulses when active. Tool count next to each MCP.

**images** — last 4 images claude saw or generated. Tiny SVG thumbs. `↵ view full` opens a modal with the actual image at native resolution.

**notification** — slides up when claude needs you. One at a time visible; queue indicator if more pending. Distinct color for who's asking (coral pink = claude, purple = clyde itself e.g. `clyde lost connection to the session`).

**status bar** — keybind hints + version + ready state.

---

## visual language

### palette (tokyo night–ish)

| token | hex | use |
|---|---|---|
| `bg` | `#15151f` | terminal surface |
| `surface` | `#1a1b26` | panel bg (with .55 alpha) |
| `border-dim` | `#232540` | default panel border |
| `border-acc` | `#7c5cff` | active/focused panel |
| `text` | `#c0caf5` | primary text |
| `text-mid` | `#a9b1d6` | secondary text |
| `text-dim` | `#565f89` | metadata, labels |
| `text-fade` | `#3b4261` | gutters, disabled |
| `purple` | `#bb9af7` | clyde brand, active substate |
| `pink` | `#ff75a0` | mascot, claude voice |
| `cyan` | `#7dcfff` | paths, info |
| `green` | `#9ece6a` | live, done, success |
| `amber` | `#e0af68` | modified, warnings |
| `red` | `#f7768e` | removed, errors, current file |
| `orange` | `#ff9e64` | grep / find tags |

### typography

JetBrains Mono / IBM Plex Mono / SF Mono fallback. Base 11.5px, panel labels 11px, mascot 12.5px, branding 13px tracked at .22em.

### animations

- **mascot eyes** — 4.5s loop, eyes go transparent for ~4% of the cycle (sharp blink, not a fade)
- **mascot body** — 3.5s sinusoidal bob, 2px amplitude
- **live dot** — pulsing halo via box-shadow ring, 1.6s
- **progress bars** — gradient fill + shimmer overlay sliding right every 2.5s
- **active substep cursor** — `▎` blinking on the active line via `step-end`
- **spinner** — single `◐` glyph rotating 360° per second
- **notification entry** — translateY(8px → 0) + opacity, 0.6s ease-out
- **active task chevron** — opacity pulse 1.4s

### borders & corners

- 1px borders, 4px panel radius
- panel labels overlap the top border on the left, metadata badge on the right (both with bg matching the outer surface to look "cut into" the border)
- dotted dividers (`1px dotted #1f2335`) for sub-sections within a panel
- dashed for major separators (header / notification / statusbar)

---

## feature backlog (brainstorm — not yet designed)

### explorer
- click file → open read-only viewer (right pane swap, or modal)
- `⌘e` → in-clyde edit buffer with vim-ish bindings
- `/` → fuzzy find filter
- right-click → reveal in editor / copy path / git blame
- show git status more granularly (untracked, staged, conflicted)
- collapse/expand directories with `←` `→`
- `g` jump to active file in tree

### images
- `↵` → modal viewer with the full image, native resolution, zoom + pan
- show metadata: dims, size, source (claude generated / user uploaded / screenshot tool)
- pin important ones so they survive the FIFO eviction
- copy as base64 / save to disk
- if the terminal supports kitty graphics protocol or iTerm2 inline images, render the thumbs as actual pixels not SVG approximations

### tasks
- drag pending tasks to reorder (insert priority)
- click any task to expand/collapse substeps
- right-click → skip / edit / split / mark blocked
- show task created + completed timestamps on hover
- collapse the entire `done` section into one line (`✓ 3 done · click to expand`)
- if claude pivots and abandons a task, mark it `⊘ skipped` with reason

### diff
- click a hunk → jump to that file/line in your editor (LSP "open" command)
- stage/unstage individual hunks (writes through to git)
- side-by-side mode toggle
- per-language syntax highlighting
- "since checkpoint" mode — diff vs a snapshot point, not just current changes

### notifications
- stack — show top one expanded, others as compact pills
- system notification + sound when clyde's window isn't focused
- per-action-type policies: `auto-approve reads · auto-approve installs in node_modules · always ask for bash`
- show full proposed command, not just tool + arg
- timeout option: auto-deny after N seconds with explanation back to claude

### states (frame-level)
- **idle** — claude not running. mascot relaxed (closed eyes? `_`), title bar dim, "session ended"
- **paused / awaiting input** — purple frame tint, mascot eyes track sideways `◔ ◔`, status "waiting on you"
- **error** — red flash on the panel that errored, mascot eyes wide `◉ ◉` then settle, status text shows the error
- **success / task complete** — brief green pulse on the tasks panel border + mascot smile

### multi-session
- tab strip in title bar — switch between projects clyde is watching
- "all sessions" overview mode — small cards for each, current task headline only
- aggregate cost across sessions

### settings
- show/hide individual panels
- reorder columns
- theme picker (tokyo night, catppuccin, gruvbox, dracula, custom)
- mascot variants — and a "no mascot" mode for the unsentimental
- font picker
- compact vs spacious density

### keyboard
- `⌃k` palette — fuzzy command runner
- `tab` / `shift-tab` cycle panel focus
- `j` `k` within a panel
- `?` cheatsheet overlay

---

## open questions

- **transport** — how does clyde get data from claude? Read claude's logs? IPC? A small companion server claude posts to? An MCP that claude reports its own state through? Probably a local socket where claude writes structured events.
- **task source of truth** — pull from `TodoWrite` tool calls verbatim, or also infer steps from the conversation flow? `TodoWrite` is reliable but not always present.
- **diff freshness** — file watcher (fsnotify) on the working tree, or hook into claude's edit events directly so we can show pending edits before they hit disk?
- **multi-instance** — one clyde per project, or one clyde watching all of them?
- **does clyde have its own LLM affordances**, or is it strictly an observer? (Inline AI quick-actions in the diff would be useful but blur the role.)
- **terminal vs native** — pure TUI (Bubble Tea / Textual / Ratatui) or a "TUI-styled" native window? Pure TUI is more authentic and ssh-friendly; native lets the image viewer actually display images without graphics-protocol contortions.

---

## implementation likely stack

- **TUI**: Bubble Tea + Lip Gloss (Go), or Textual (Python), or Ratatui (Rust)
- **file watching**: fsnotify / watchman
- **git**: shell out to `git` for diffs, status, hunks
- **claude link**: local UDS socket where claude code writes structured events (JSONL); clyde tails it
- **image protocol**: Kitty graphics protocol primary, iTerm2 inline images fallback, half-block character fallback for legacy terminals
- **syntax highlighting**: tree-sitter or chroma
