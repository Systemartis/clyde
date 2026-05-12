---
name: improve-codebase-architecture
description: Use when refactoring, adding a new adapter/port, or when a file passes ~500 LOC of branching logic. Identifies shallow modules and proposes deepening opportunities using the Ousterhout vocabulary (depth, seam, leverage). Adapted from skills.sh/mattpocock/skills/improve-codebase-architecture for clyde's hexagonal layout.
---

# improve-codebase-architecture (clyde-tuned)

Goal: convert shallow modules (small interface, similar-sized implementation) into deep ones (tight interface, large implementation hidden behind it). Deeper modules give callers leverage and concentrate the cost of change in one place.

## Vocabulary (use exactly)

- **Module**: any unit with an interface + implementation — a function, a package, a port, an adapter.
- **Interface**: everything callers need to know — types, invariants, error modes, ordering, config.
- **Implementation**: internal code.
- **Depth** = behavior / interface complexity. Higher is better.
- **Seam**: where an interface exists; where behavior can be replaced without editing in place.
- **Adapter**: a concrete implementation at a seam.
- **Leverage**: value callers gain from depth.
- **Locality**: change, bugs, knowledge concentrate in one place.

## Tests

- **Deletion test:** if you remove the module, does complexity disappear or just spread to N callers? If it spreads, the module was load-bearing — keep it. If it disappears, the module was shallow rubble — inline it.
- **The interface IS the test surface.** If you can only test through internal hooks, the interface is leaky.
- **One adapter = hypothetical seam. Two adapters = real seam.** Don't introduce a `port` until two adapters exist.

## Three-phase process

### 1. Explore

Walk the affected area without a checklist. Record friction:

- Bouncing between files to understand one behavior
- Long argument lists / option structs
- "Util" / "helper" / "common" packages (often shallow)
- Tests that mock the unit under test
- Files where adding a feature means touching three packages

### 2. Present candidates

For each opportunity, write:

- **Files involved** (file:line where the friction is)
- **Problem statement** in one sentence
- **Solution sketch** in plain English — no final API yet
- **Benefits** framed around locality (one file changes) and leverage (callers do less)

### 3. Grilling loop

Iterate on the design with a peer (or the user) before writing code. Update `openspec/specs/<layer>/...` if the change affects an invariant.

## clyde-specific friction list (as of 2026-05-05)

Use these as starting points; verify before acting.

### TUI keys.go (1328 LOC) — shallow dispatch

`internal/adapters/tui/keys.go` is a giant switch over `(panel, key)` pairs. Adding a keybinding means editing the bottom of the file. Likely deepening: extract a `keymap` package keyed by `(focusedPanel, key) → Intent`, and let `model.Update` dispatch on `Intent` instead of raw key strings. The interface shrinks from "1328 lines of cases" to "Intent + dispatch table."

### TUI panel_viewer.go (1092 LOC) + viewer_edit_commands.go (529)

These two files implement the file viewer's edit mode. The seam between "view" and "edit" is implicit. Deepening: introduce a `Viewer` interface with two adapters (read-only, editable). The deletion test confirms this — removing the boundary today means edit-mode logic leaks into rendering.

### `internal/adapters/git` shared cache

`Source` is *already* a deep module: `Status`, `Branch`, `Diff` share one TTL cache, and the cache is invisible to callers. Note this as a positive example — copy the shape elsewhere.

### `internal/adapters/claudesettings`

Single-file Reader with `(mtime, size)` cache. Two consumers (`mcpconfig`, `lspscan`) — passes the "two adapters = real seam" test. Another positive example.

### `cmd/clyde/main.go` (175 LOC of wiring)

Wiring is fine here — composition root. But the conditional fallback chain (cannot-init-source → demo, cannot-getwd → demo, hookserver-fails → continue) is hard to follow. Could deepen via `livesession.Builder` that absorbs the fallback decisions.

## Anti-patterns to flag in review

- A new `helpers.go` or `util.go` file
- A port with one method that's only ever called from one site
- A test that imports `testing/quick` only to construct a struct (means the public ctor is too narrow)
- A new "config" struct with >5 boolean fields
- Adapter that imports `internal/domain/...` types directly *and* mutates them (adapters convert; they do not own domain state)

## Output format

Add findings to `analysis/ARCHITECTURE-FINDINGS.md` (created on first use) with:

```
### <Title>
- **Files:** <paths:lines>
- **Problem:** <one sentence>
- **Sketch:** <plain English, no API yet>
- **Benefits:** locality / leverage / testability
- **Deletion test:** <what happens if we delete this module today>
```
