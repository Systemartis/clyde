# TUI Spec: Behavior

## Overview

The TUI adapter presents the V1 walking skeleton: a single-pane view showing the last N Events from the focused Session in the current Project, with quit behavior via keyboard.

## Scenarios

### Scenario: Startup Display

**Given** a Project with at least one Session containing Events

**When** the TUI is launched

**Then** the TUI MUST display the last N Events (default N = 5) of the most-recently-active Session

**And** each Event MUST show its timestamp and kind

### Scenario: Event Rendering Format

**Given** an Event with a timestamp and kind

**When** the TUI renders that Event

**Then** the rendered line MUST include the Event's timestamp (e.g., HH:MM:SSZ format)

**And** the rendered line MUST include the Event's kind identifier

**And** for known kinds (user, assistant), the kind SHALL be displayed as-is

**And** for unknown kinds, the kind SHALL be displayed with a notation indicating it is opaque

### Scenario: Empty State

**Given** a Project with no Sessions

**When** the TUI is launched

**Then** the TUI MUST render without error

**And** the TUI SHOULD display an empty or placeholder state (e.g., "No sessions found" message)

### Scenario: Quit via q key

**Given** the TUI is running

**When** the user presses `q`

**Then** the TUI MUST exit cleanly with no error

**And** the program exit code MUST be 0

### Scenario: Quit via ctrl+c

**Given** the TUI is running

**When** the user presses `ctrl+c`

**Then** the TUI MUST exit cleanly with no error

**And** the program exit code MUST be 0

## Implementation Notes

- The TUI uses Bubble Tea v2 for rendering and event handling.
- The Model type implements Tea's model interface (Init, Update, View).
- Event line format: `HH:MM:SSZ <kind> <summary>` (timestamp in UTC, kind identifier, and opaque indicator if unknown).
- The TUI delegates data retrieval to the WatchSession use case via a composed instance.
- Window resize events are handled gracefully (no panic, no loss of data).

## Out of Scope for V1

- Multi-pane layout or navigation
- Mouse support or image rendering
- Interactive focus switching between Sessions (deferred to `sessions-list-panel`)
- Token cost or usage panel (deferred to `usage-and-cost-panel`)
