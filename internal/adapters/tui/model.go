// Package tui provides the Bubble Tea v2 TUI adapter for clyde.
// Real data is wired via the LiveSession use case when --demo is not specified.
// Run cmd/clyde to start the TUI; pass --demo for deterministic mock mode.
package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/clyde-tui/clyde/internal/adapters/hookserver"
	"github.com/clyde-tui/clyde/internal/application/livesession"
	"github.com/clyde-tui/clyde/internal/domain/project"
	"github.com/clyde-tui/clyde/internal/ports"
)

// PanelID identifies a focusable panel.
type PanelID int

// Panel ID constants. Tab cycles through these in order.
const (
	PanelNow PanelID = iota
	PanelCalls
	PanelDiff
	PanelUsage
	PanelExplorer
	PanelServers
	// PanelBash — session-wide ledger of every Bash command claude has run,
	// with exit/duration. Off by default; opt-in via the settings overlay
	// per project.
	PanelBash
	// PanelCache — prompt-cache efficiency dashboard (hit ratio, totals,
	// biggest cache miss, per-turn trend sparkline). Off by default.
	PanelCache
	panelCount // sentinel — keep last
	// PanelNone is the zero-like sentinel meaning "no panel is active".
	// Defined after panelCount so it's never confused with a real panel.
	PanelNone PanelID = -1
)

// CompactionState describes how close a session is to the context window limit.
type CompactionState int

const (
	// CompactionOK — tokens < 75% of context limit. No warning shown.
	CompactionOK CompactionState = iota
	// CompactionWarn — tokens 75–90% of context limit. Inline warning in usage panel.
	CompactionWarn
	// CompactionDanger — tokens > 90% of context limit. Danger indicator + notification banner.
	CompactionDanger
)

// compactionWarnPct is the token percentage threshold for CompactionWarn.
const compactionWarnPct = 75

// compactionDangerPct is the token percentage threshold for CompactionDanger.
const compactionDangerPct = 90

// deriveCompactionState computes a CompactionState from a token percentage (0–100).
func deriveCompactionState(tokenPct int) CompactionState {
	switch {
	case tokenPct >= compactionDangerPct:
		return CompactionDanger
	case tokenPct >= compactionWarnPct:
		return CompactionWarn
	default:
		return CompactionOK
	}
}

// Model is the root Bubble Tea v2 model for the clyde prototype.
type Model struct {
	width  int
	height int
	bp     Breakpoint

	palette Palette
	styles  Styles
	data    MockData
	// cfg is the EFFECTIVE config — base merged with the per-project
	// override for liveProject.CWD(). Render-time filters read this.
	cfg Config
	// baseCfg is the raw file contents (global panels + every project
	// override). The settings overlay edits this; cfg is recomputed
	// from baseCfg.EffectiveFor(cwd) on every save.
	baseCfg Config
	keymap  KeyMap

	// Layout mode: stack, tabs, or multi-col
	layoutMode LayoutMode

	focused  PanelID
	frame    FrameState
	quitting bool

	// Collapse state + harmonica springs — one per panel
	collapse [panelCount]PanelCollapseState

	// Three-state interaction model (v11):
	//   State 1: Collapsed
	//   State 2: Expanded-Passive (expanded, not entered)
	//   State 3: Expanded-Active (entered — scroll & resize active)
	// activePanelID holds the panel currently in State 3.
	// PanelNone when no panel is in active mode.
	activePanelID PanelID

	// bubbles/v2/progress bars — one per animated bar
	// progTokens: token / time usage (Green→Amber→Red gradient)
	// progReset:  next reset countdown (Cyan→Purple gradient, distinct)
	progTokens progress.Model
	progReset  progress.Model

	// notification slide-in position (lerp)
	notifPos float64 // 0.0 = hidden (offscreen), 1.0 = fully visible
	// notifAck: true when the user dismissed the notification (Esc)
	notifAck bool

	// Explorer interactive state
	explorer ExplorerState

	// File viewer state machine
	viewerActive bool   // true when viewer occupies the right column
	viewerFile   string // path of the currently open file (relative, matches mock keys)
	viewport     ViewerViewport
	// viewerDiffLines is a per-viewer-file diff loaded synchronously when
	// the user opens a file from the explorer. Without this the viewer
	// would only have green/red highlights for the file claude is
	// currently editing (m.data.DiffLines, scoped to v.DiffFile) — every
	// other modified file would render plain even though git knows their
	// hunks.
	viewerDiffFile  string
	viewerDiffLines []DiffLine
	// viewerCachedFile / viewerCachedContent / viewerCachedSize /
	// viewerCachedHL memoise the read+tokenise pipeline for the open
	// file. Without this the renderer was re-reading from disk and
	// re-running chroma's tokeniser on EVERY frame, which makes scroll
	// stutter on multi-thousand-line files (chroma alone is 50-200 ms
	// for a 5k-line Go file). Cache invalidates only when viewerFile
	// changes — not on disk-side edits, which the user can still pick
	// up by closing + reopening the file.
	viewerCachedFile    string
	viewerCachedContent string
	viewerCachedSize    int64
	viewerCachedHL      []string
	// viewerMode chooses between read-only navigation (ViewerView) and
	// edit (ViewerEdit). View is the default after open; `i` enters
	// Edit and Esc returns. The dispatcher and the bottom-bar hint
	// branch on this so each mode shows the right keybinds.
	viewerMode ViewerMode
	// viewerEdit holds the mutable buffer for the currently-open file.
	// Lazy-init: zero-valued until the user enters edit mode for the
	// first time on a given file. Mutated in place by edit ops; the
	// Model's value-typed shape is preserved by Go's slice header
	// assignment (Lines is a slice, so copying the struct still shares
	// backing storage — we explicitly re-assign on every mutation
	// helper to keep that contract honest).
	viewerEdit editBuffer
	// viewerDirty is true when the buffer has unsaved changes. Drives
	// the "*" indicator in the viewer's title and the "[+]" mode badge,
	// matching vim's convention.
	viewerDirty bool
	// viewerStatus is a transient one-line message rendered on the
	// viewer's hint row when non-empty (e.g. "saved to <path>" after
	// Ctrl+S). Cleared on the next keystroke or after a short timer
	// upstream — for now we just clear it on the next status-changing
	// action.
	viewerStatus string
	// viewerCmdActive is true while the user is composing a `:command`
	// in the editor. Enters via `:` from view mode; the bottom hint row
	// turns into a prompt where keystrokes append to viewerCmdBuf.
	// Enter executes (parses + dispatches via runViewerCommand); Esc
	// cancels.
	viewerCmdActive bool
	viewerCmdBuf    string
	// viewerHistory and viewerRedo are the undo / redo stacks for the
	// edit buffer. We snapshot the buffer state (lines + cursor) BEFORE
	// each mutating operation while in edit mode; undo pops the most
	// recent snapshot, redo re-applies what undo just popped. Bounded
	// at maxViewerHistory entries to keep memory predictable on long
	// editing sessions.
	viewerHistory []editBuffer
	viewerRedo    []editBuffer
	// viewerFind* drive the in-viewer find prompt (`/<query>` from view
	// mode). FindActive is true while the prompt is open; FindQuery
	// holds what the user has typed; FindReplace is an optional
	// replacement string the user types in the second field of the
	// prompt (Tab toggles which field is focused); FindMatches is the
	// list of (line, col) hits computed when Enter executes the search;
	// FindIdx tracks which match is currently focused so n/N can step
	// through; FindFocusReplace flags which input the keystrokes flow
	// into while the prompt is open.
	viewerFindActive       bool
	viewerFindQuery        string
	viewerFindReplace      string
	viewerFindFocusReplace bool
	viewerFindMatches      []findMatch
	viewerFindIdx          int
	// viewerSelActive + viewerSelAnchor drive the keyboard-driven text
	// selection in edit mode. Anchor is set when the user first holds
	// Shift while moving; subsequent shifted motions extend the
	// selection from anchor to cursor. Plain (non-shifted) motion
	// clears the selection. ⌃c / ⌘c copies the range to the system
	// clipboard via OSC 52.
	viewerSelActive bool
	viewerSelAnchor cursorPos
	// viewerFullscreen toggles the viewer between "right-column overlay"
	// (default — explorer + servers stay visible) and "takes the whole
	// screen between title bar + status bar". clyde is designed to sit in
	// a side pane next to claude, so the horizontal cells eaten by the
	// explorer column are real estate the user wants back when reading
	// or editing a file. Toggled with `f`; reset to false on viewer close.
	viewerFullscreen bool
	// vimGPending tracks whether the user just pressed `g`. The next `g`
	// triggers `gg` (jump to top); any other key clears the flag. Lives
	// here rather than inside ViewerViewport so the rest of the viewport
	// state stays a pure value type.
	vimGPending bool

	// Per-panel custom heights set by +/- resize (0 = use spring height).
	panelHeights map[PanelID]int

	// Per-panel viewports for scrollable content (tasks, diff).
	// In Expanded-Active state, ↑/↓ scrolls the panel's viewport.
	// In Expanded-Passive or Collapsed, viewport offset is ignored / reset.
	// Explorer is a special case: ↑/↓ moves tree highlight instead.
	panelVPs [panelCount]viewport.Model

	// ── Live data (Phase A wire-up) ───────────────────────────────────────────
	// liveSession is non-nil when running in real-data mode (--demo not set).
	// When nil, the model runs in demo mode using only data (MockData).
	liveSession *livesession.LiveSession

	// liveProject is the project we're watching in live mode.
	liveProject project.Project

	// liveView is the latest snapshot from the LiveSession use case.
	// It is replaced on every liveSessionMsg. Zero value is safe (empty state).
	liveView livesession.View

	// demoMode is true when the user passed --demo. When true, liveSession is nil
	// and only MockData is used (deterministic, good for golden tests).
	demoMode bool

	// liveIsGitRepo is true when the live cwd is inside a git repository.
	// Updated on every snapshotCmd result and forwarded to data.IsGitRepo
	// in applyLiveView. Always false in demo mode (demo sets IsGitRepo via MockData).
	liveIsGitRepo bool

	// prevLatestEventID is the ID of the latest event seen in the previous
	// liveSessionMsg. Used to detect new events and trigger mascot reactions.
	prevLatestEventID string

	// prevEventCount is the event count seen in the previous liveSessionMsg.
	// Tracked for mascot triggers — a per-tick "did the focused session
	// gain events?" hint. The older performance debounce that gated
	// applyLiveView on these counters is gone (correctness > micro-perf;
	// closed sessions used to cling to the tab strip until something else
	// woke the re-apply path up).
	prevEventCount int

	// sessionTabIndex selects which session-tab the user has focused.
	//   -1  → Σ aggregate (cwd-wide leaderboard view of usage panel)
	//    0  → most-recently-active session (default on launch)
	//    n  → recent[n] in the cwd
	// Updated by ] / [ keys via cycleSession; consumed by snapshotCmd to
	// resolve a session.ID for SnapshotForSession.
	sessionTabIndex int

	// helpOpen is true while the per-panel help overlay is showing.
	// Toggled by the `h` key (whenever — same model as `?` for settings).
	// While true, every expanded panel swaps its normal content for a
	// keybind cheat-sheet specific to that panel; the bottom status bar
	// keeps only global navigation hints (the per-panel commands are
	// already redundant once each panel surfaces its own).
	helpOpen bool

	// diffSrc is the source for git diff hunks (Phase E).
	// Non-nil in live mode; nil in demo mode (mock diff data is kept).
	diffSrc DiffSource

	// ── Hook server (Phase H) ─────────────────────────────────────────────────
	// hookServer is non-nil in live mode only. Its lifecycle is tied to the
	// Bubble Tea program — the server shuts down when the program exits.
	hookServer *hookserver.Server

	// hookNotif holds the currently-pending hook event for the notification banner.
	// Zero value means no pending event (banner shows demo data or is hidden).
	hookNotif HookNotification

	// hookPendingCh is the ResponseCh of the in-flight hook event. Non-nil when
	// hookNotif.Active is true. The model sends to this channel when the user
	// presses y or n, then clears hookNotif.
	hookPendingCh chan hookserver.HookResponse

	// compaction holds the current context-window saturation state.
	// Recomputed on every liveSessionMsg from data.TokenPct.
	compaction CompactionState

	// ── Settings overlay (Phase I.4) ─────────────────────────────────────────
	// settingsOpen is true when the settings modal is visible.
	settingsOpen bool
	// settingsToggles is the current panel-visibility toggle state shown in
	// the overlay. Populated from cfg when the overlay is opened.
	settingsToggles []SettingsPanelToggle
	// settingsCursor is the highlighted row index in settingsToggles.
	settingsCursor int
	// settingsScope chooses which layer of the config the overlay edits.
	// Project (default) writes to [projects."<cwd>"]; Global writes to the
	// top-level [panels] table.
	settingsScope SettingsScope
	// settingsRememberLayout mirrors baseCfg.RememberLayout while the
	// overlay is open. Committed back to baseCfg on Esc; toggled via
	// Enter when cursor == settingsRememberLayoutCursor.
	settingsRememberLayout bool
	// settingsNotificationStyle mirrors baseCfg.NotificationStyle while
	// the overlay is open. Committed back to baseCfg on Esc; cycled via
	// Enter when cursor == settingsNotificationCursor.
	settingsNotificationStyle NotificationStyle
	// settingsCostThreshold mirrors baseCfg.NotifyCostThresholdUSD while
	// the overlay is open. Committed back to baseCfg on Esc; cycled via
	// Enter when cursor == settingsCostThresholdCursor.
	settingsCostThreshold float64
	// settingsTheme mirrors baseCfg.Theme while the overlay is open.
	// Cycled by Enter on settingsThemeCursor; the model.applyTheme is
	// called immediately so the user sees the new palette behind the
	// settings card before they Esc out and persist.
	settingsTheme Theme
	// settingsMascotPersona mirrors baseCfg.MascotPersona while open.
	settingsMascotPersona MascotPersona
	// settingsBootScreenEnabled mirrors baseCfg.BootScreenEnabled while open.
	settingsBootScreenEnabled bool

	// demoNotificationFired records whether the user has used the
	// Ctrl+N preview trigger in this session. We only let it fire ONCE
	// — repeated presses no-op so the binding can't be spammed and the
	// chrome doesn't flicker if the user holds the chord. Reset on
	// process restart, not on dismiss.
	demoNotificationFired bool

	// quotaNotif is the active plan-quota / cost alert (if any). Set
	// by evaluateQuotaThresholds when a fresh threshold crossing is
	// detected; consumed by resolveNotification alongside hook +
	// compaction.
	quotaNotif QuotaNotification

	// quotaFired latches per-threshold crossings so the same alert
	// doesn't re-fire on every plan-usage poll. Cleared per-threshold
	// when utilization drops below the hysteresis low-water mark.
	quotaFired map[quotaFireKey]bool

	// boot drives the animated startup splash. Active is set in Init when
	// cfg.BootScreenEnabled is true; cleared when the splash finishes or
	// the user dismisses it with any key. View() routes to renderBootScreen
	// while Active is true.
	boot BootScreen

	// lastInteractionTick is the FrameMsg tick at which the user last
	// pressed a key, clicked, or scrolled. Drives visualFocus(pid): after
	// focusFadeStartTick of inactivity the focus highlight fades off the
	// keyboard-target panel and reappears on PanelNow so the mascot
	// becomes the calm landmark when the user comes back from claude.
	// Initialized to 0 so a fresh launch starts in the "active" window.
	lastInteractionTick uint64

	// tickGen is the current generation of the FrameMsg tick chain.
	// Bumped by markInteraction (and by every successful handleFrame) so
	// that switching from idle (1Hz) to active (20Hz) on a keypress
	// invalidates any pending slow tick. handleFrame drops FrameMsgs
	// whose Gen does not match this counter — see frame.go for the rest
	// of the contract.
	tickGen uint64

	// llmSource is the name of the LLM CLI source driving this view,
	// e.g. "claude-code". Set in NewModelLive from --source flag.
	// Displayed in the title bar: "clyde · claude-code · ~/path · ...".
	llmSource string

	// copyToast is a transient status-bar message shown after a yank
	// (y / Y or double-click in explorer). Empty = no toast. Cleared by
	// a clearCopyToastMsg fired ~1.5s after the toast was set; a fresh
	// yank in that window simply replaces the text and resets the timer.
	copyToast string
	// copyToastExpires is the absolute moment after which the toast may
	// be cleared by a tick. We compare against this in the handler so a
	// pending tick from an older toast cannot wipe a newer one early.
	copyToastExpires time.Time

	// lastExplorerClickPath / lastExplorerClickAt track the most recent
	// left-click on an explorer row so we can detect a double-click
	// (same row within explorerDoubleClickWindow) and route it to copy
	// instead of the single-click open behavior.
	lastExplorerClickPath string
	lastExplorerClickAt   time.Time

	// lastPanelClickPanel / lastPanelClickAt track the most recent
	// left-click on a panel header / empty area (i.e. a click that
	// only focuses, not one that hits an explorer row). A second click
	// on the SAME panel within panelDoubleClickWindow promotes the
	// focus to Expanded-Active mode — gives mouse users a way to enter
	// active mode without reaching for the keyboard. Right-click is
	// deliberately not used because many terminals intercept it.
	lastPanelClickPanel PanelID
	lastPanelClickAt    time.Time

	// ── Plan-usage adapter (real numbers from Anthropic's settings API) ──────
	// planUsageSrc is the optional plan-quota source. Nil in demo mode and
	// when the live wire-up cannot find OAuth credentials. When non-nil, the
	// model fetches every planUsageRefreshInterval and merges the returned
	// percentages into the usage panel; otherwise we fall back to the
	// time-elapsed approximation derived from JSONL data.
	planUsageSrc ports.PlanUsageSource
	// planUsage holds the most-recent successful Fetch result. Zero value
	// means "no data yet"; check PlanWindow.Present before using fields.
	planUsage ports.PlanUsage
	// planUsageErr is the last fetch error. Non-nil means we are showing
	// stale or fallback data; the panel surfaces a small "(plan offline)"
	// badge so the user knows the percentages may not be live.
	planUsageErr error
}

// liveRefreshInterval is the period between automatic LiveSession snapshots.
const liveRefreshInterval = 1 * time.Second

// planUsageRefreshInterval is the period between automatic Anthropic
// plan-quota fetches. 5 min mirrors the cadence codexbar uses — the data
// itself only ticks meaningfully every few minutes, and we don't want to
// hammer the endpoint.
const planUsageRefreshInterval = 5 * time.Minute

// NewModel constructs a Model with mock data and the Tokyo Night v4 theme.
// It runs in demo mode (demoMode=true): all data is from MockData, no live reads.
func NewModel() Model {
	return NewModelWithConfig(DefaultConfig(), LayoutStack)
}

// NewModelWithConfig constructs a Model with the given config and layout override.
// Runs in demo mode (demoMode=true).
func NewModelWithConfig(cfg Config, layoutOverride LayoutMode) Model {
	m := newBaseModel(cfg, layoutOverride)
	m.demoMode = true
	return m
}

// NewModelLive constructs a Model wired to real Claude Code data.
// It runs in live mode: on Init it fires a LiveSession.Snapshot and then
// refreshes every liveRefreshInterval. The demo data is kept as a fallback
// for panels not yet wired in Phase A (explorer, servers, diff, usage details).
//
// p is the project to watch (typically os.Getwd()).
// ls is the LiveSession use case already constructed with real adapters.
// hs is the optional hook server; pass nil to disable hook integration.
// ds is the optional DiffSource for Phase E; pass nil to keep mock diff data.
// sourceName is the LLM source name for the title bar (e.g. "claude-code").
func NewModelLive(cfg Config, layoutOverride LayoutMode, p project.Project, ls *livesession.LiveSession, hs *hookserver.Server, ds DiffSource, sourceName string) Model {
	// Apply per-project config overrides (Phase 4) — cfg as passed in is
	// the raw global config; the effective render-time config merges in
	// any [projects."<cwd>"] overrides.
	effective := cfg.EffectiveFor(p.CWD())
	m := newBaseModel(effective, layoutOverride)
	m.baseCfg = cfg // keep raw for the settings overlay to edit
	m.demoMode = false
	m.liveProject = p
	m.liveSession = ls
	m.hookServer = hs
	m.diffSrc = ds
	m.llmSource = sourceName
	return m
}

// isClaudeWorking returns true when the now-panel mode-text indicates
// claude is actively producing output: tool running, thinking, writing.
// The dedicated idle / awaiting / responded / thought labels return false
// — those mean claude has handed control back. Drives the mascot FSM's
// working hint so the kitten reads as alert during real activity instead
// of standing idle while the model streams a response.
func (m Model) isClaudeWorking() bool {
	mode := strings.ToLower(strings.TrimSpace(m.data.NowMode))
	if mode == "" {
		return false
	}
	switch mode {
	case "idle", "awaiting", "responded", "thought":
		return false
	}
	return true
}

// Focus decay timings (in FrameMsg ticks at the 50ms tick interval).
//
// Two-phase smooth handoff once the user has been idle in the dashboard:
//
//	0..focusFadeStartTick    — alpha=1.0 on the keyboard-target panel
//	start..focusFadeMidTick  — m.focused alpha 1→0 over 3s (purple → gray)
//	mid..focusFadeEndTick    — PanelNow  alpha 0→1 over 3s (gray → purple)
//	end..                    — alpha=1.0 on PanelNow (mascot is the landmark)
//
// The 30s idle threshold is long enough that the user reading a long
// activity log doesn't see the chrome shift; the 6s total transition is
// slow enough that the color blend is observable rather than a snap.
const (
	focusFadeStartTick = 600 // 30s — fade-out begins
	focusFadeMidTick   = 660 // 33s — fade-out done, fade-in begins
	focusFadeEndTick   = 720 // 36s — settled on the mascot
)

// idleTicks returns how many FrameMsg ticks have elapsed since the last
// keyboard / mouse interaction. Returns 0 right after any interaction.
func (m Model) idleTicks() uint64 {
	if m.frame.Tick < m.lastInteractionTick {
		return 0
	}
	return m.frame.Tick - m.lastInteractionTick
}

// isDashboardMode reports whether clyde is currently acting as a passive
// dashboard rather than a foregrounded modal/editor. Focus decay only
// runs in dashboard mode — when the user has the settings overlay, the
// file viewer, the help cheat-sheets, an active hook prompt, or the boot
// splash on screen, the keyboard-target panel keeps its full purple
// border because the user IS interacting with clyde, just not with the
// panel grid.
func (m Model) isDashboardMode() bool {
	switch {
	case m.settingsOpen:
		return false
	case m.viewerActive:
		return false
	case m.helpOpen:
		return false
	case m.hookNotif.Active:
		return false
	case m.boot.Active:
		return false
	}
	return true
}

// focusAlpha returns 0..1 representing how "focused" pid should look at
// the current idle tick. 1.0 is full focus chrome (purple border), 0.0 is
// fully unfocused, fractional values are smooth interpolations during the
// handoff window. Only computes a non-binary value in dashboard mode —
// outside dashboard mode it snaps to 0 or 1 so an open overlay sees a
// stable highlight on the keyboard-target panel.
func (m Model) focusAlpha(pid PanelID) float64 {
	if !m.isDashboardMode() {
		if pid == m.focused {
			return 1.0
		}
		return 0.0
	}
	idle := m.idleTicks()

	// Active phase — the keyboard-target panel owns the highlight.
	if idle < focusFadeStartTick {
		if pid == m.focused {
			return 1.0
		}
		return 0.0
	}

	// Fade-out: m.focused dims toward 0 while everyone else stays at 0.
	if idle < focusFadeMidTick {
		if pid != m.focused {
			return 0.0
		}
		span := float64(focusFadeMidTick - focusFadeStartTick)
		progress := float64(idle-focusFadeStartTick) / span
		return 1.0 - progress
	}

	// Fade-in: PanelNow brightens toward 1, others stay at 0.
	if idle < focusFadeEndTick {
		if pid != PanelNow {
			return 0.0
		}
		span := float64(focusFadeEndTick - focusFadeMidTick)
		progress := float64(idle-focusFadeMidTick) / span
		return progress
	}

	// Settled — mascot owns the highlight.
	if pid == PanelNow {
		return 1.0
	}
	return 0.0
}

// visualFocus returns whether pid should render with the focus-passive
// border treatment (Passive chrome). True whenever focusAlpha is strictly
// positive, so during a fade the panel keeps its label/state classification
// while the border color blends smoothly via WithFadedFocus.
func (m Model) visualFocus(pid PanelID) bool {
	return m.focusAlpha(pid) > 0
}

// markInteraction resets the idle-fade timer AND bumps the tick
// generation, returning a wakeup tea.Cmd the caller MUST batch with the
// rest of its return value.
//
// The wakeup ensures that an interaction during idle (1Hz) mode flips
// the loop back to fast (20Hz) immediately rather than waiting up to a
// second for the next slow tick. The gen bump invalidates any pending
// slow tick scheduled by the prior chain.
func (m *Model) markInteraction() tea.Cmd {
	m.lastInteractionTick = m.frame.Tick
	m.tickGen++
	return tickCmd(m.tickGen, activeTickInterval)
}

// nextTickInterval picks the inter-frame gap for the next FrameMsg
// based on whether anything visible is currently animating.
func (m Model) nextTickInterval() time.Duration {
	if m.shouldFastTick() {
		return activeTickInterval
	}
	return idleTickInterval
}

// shouldFastTick returns true when at least one animation source is
// active and the View() loop must therefore re-render at activeTickInterval.
//
// Sources, in roughly the order they are likely to fire:
//   - boot splash animation in progress
//   - claude is mid-response (the now-panel "thinking/writing" indicator
//     advances per frame)
//   - mascot is playing a sequence (blink, look, wave, sleep transitions)
//   - any collapse spring is mid-flight
//   - notification slide-in not yet at rest
//   - focus-fade is still in its 36s decay window since the last input
func (m Model) shouldFastTick() bool {
	if m.boot.Active {
		return true
	}
	if m.isClaudeWorking() {
		return true
	}
	if m.frame.Mascot.HasActiveSequence() {
		return true
	}
	for i := range m.collapse {
		if !m.collapse[i].IsSettled() {
			return true
		}
	}
	if m.notifPos < 1.0 {
		return true
	}
	if m.idleTicks() < focusFadeEndTick {
		return true
	}
	return false
}

// applyTheme rebuilds palette + styles + progress bars to use the given
// theme. Called by the settings overlay when the user cycles the theme chip
// so the change shows up live without a restart.
func (m Model) applyTheme(t Theme) Model {
	if !t.IsValid() {
		t = ThemeTokyoNight
	}
	p := PaletteFor(t)
	m.palette = p
	m.styles = NewStyles(p)
	m.progTokens = progress.New(
		progress.WithColors(p.Cyan, p.Purple, p.Pink),
		progress.WithoutPercentage(),
		progress.WithFillCharacters(progress.DefaultFullCharHalfBlock, progress.DefaultEmptyCharBlock),
	)
	m.progReset = progress.New(
		progress.WithColors(p.BorderAcc, p.Purple),
		progress.WithoutPercentage(),
		progress.WithFillCharacters(progress.DefaultFullCharHalfBlock, progress.DefaultEmptyCharBlock),
	)
	return m
}

// WithPlanUsageSource attaches a ports.PlanUsageSource to the Model so the
// usage panel can show real plan-quota percentages instead of the
// time-elapsed approximation. Pass nil to disable (the panel falls back to
// the JSONL-derived display).
//
// Returns the updated Model — the caller MUST replace its reference,
// matching the pattern used by NewModelLive.
func (m Model) WithPlanUsageSource(src ports.PlanUsageSource) Model {
	m.planUsageSrc = src
	return m
}

// newBaseModel is the shared constructor for both demo and live modes.
func newBaseModel(cfg Config, layoutOverride LayoutMode) Model {
	p := PaletteFor(cfg.Theme)

	// Default expanded heights per panel (used for collapse spring initialization)
	expandedHeights := [panelCount]float64{
		PanelNow:      10,
		PanelCalls:    18, // calls panel — generous height for hierarchy list
		PanelDiff:     10,
		PanelUsage:    14,
		PanelExplorer: 20,
		PanelServers:  13, // increased to fit 4 MCPs + 3 LSPs + headers/dividers
		PanelBash:     14,
		PanelCache:    14,
	}

	// Initialize collapse springs using config defaults
	var collapse [panelCount]PanelCollapseState
	panelCfgs := [panelCount]PanelConfig{
		PanelNow:      cfg.Panels.Now,
		PanelCalls:    cfg.Panels.Calls, // calls panel uses the Tasks slot
		PanelDiff:     cfg.Panels.Diff,
		PanelUsage:    cfg.Panels.Usage,
		PanelExplorer: cfg.Panels.Explorer,
		PanelServers:  cfg.Panels.Servers,
		PanelBash:     cfg.Panels.Bash,
		PanelCache:    cfg.Panels.Cache,
	}
	panelHeights := map[PanelID]int{}
	for i := PanelID(0); i < panelCount; i++ {
		pcfg := panelCfgs[i]
		startH := expandedHeights[i]
		// When RememberLayout is on and the user previously persisted a
		// custom height for this panel, seed the spring + override map so
		// the panel restores at exactly that size.
		if cfg.RememberLayout && pcfg.Height > 0 {
			startH = float64(pcfg.Height)
			panelHeights[i] = pcfg.Height
		}
		collapse[i] = NewPanelCollapseState(pcfg.DefaultCollapsed, startH)
	}

	// Focused panel starts expanded
	collapse[PanelNow].Expand()

	mode := layoutOverride
	if mode == "" {
		mode = cfg.Layout.DefaultMode
	}

	// Progress bars — gradient colors are sourced from the active palette
	// (Tokyo Night) so the bars stay coherent with the rest of the UI.
	//
	// progTokens: cyan → purple → pink — used for session ctx, 5h usage,
	// weekly usage. Cool→warm fill: empty reads as cyan (safe), full reads
	// as pink (Tokyo Night's warning accent). Avoids the stoplight
	// green/amber/red clash against the rest of the palette.
	progTokens := progress.New(
		progress.WithColors(p.Cyan, p.Purple, p.Pink),
		progress.WithoutPercentage(),
		progress.WithFillCharacters(progress.DefaultFullCharHalfBlock, progress.DefaultEmptyCharBlock),
	)

	// progReset: muted purple monochrome (BorderAcc → Purple). Distinct from
	// the cyan→purple→pink token gradient without screaming for attention —
	// the reset bar reports time passing, not consumption headroom.
	progReset := progress.New(
		progress.WithColors(p.BorderAcc, p.Purple),
		progress.WithoutPercentage(),
		progress.WithFillCharacters(progress.DefaultFullCharHalfBlock, progress.DefaultEmptyCharBlock),
	)

	// Per-panel viewports for scrollable content (tasks, diff).
	// Width/height are updated on first render; default to something reasonable.
	// SoftWrap is disabled: content lines are pre-rendered at the correct inner
	// width by buildXxxViewportContent, so no re-wrapping is needed or desired.
	var panelVPs [panelCount]viewport.Model
	for i := PanelID(0); i < panelCount; i++ {
		vp := viewport.New(viewport.WithWidth(76), viewport.WithHeight(10))
		vp.SoftWrap = false
		panelVPs[i] = vp
	}

	return Model{
		width:      180,
		height:     50,
		bp:         BreakpointWide,
		palette:    p,
		styles:     NewStyles(p),
		data:       V3MockData(),
		cfg:        cfg,
		baseCfg:    cfg,
		keymap:     DefaultKeyMap(),
		layoutMode: mode,
		// Default focus skips PanelNow — it's non-selectable (no scroll,
		// no actions) and would land the user on a "stuck" panel on
		// startup. PanelCalls is the natural anchor: it's the activity
		// stream the user spends most time scanning.
		focused:       PanelCalls,
		activePanelID: PanelNone,
		frame:         InitFrameState(),
		collapse:      collapse,
		progTokens:    progTokens,
		progReset:     progReset,
		notifPos:      1.0, // start visible for mock
		explorer:      NewExplorerState(V3MockData()),
		viewport:      NewViewerViewport(),
		panelHeights:  panelHeights,
		panelVPs:      panelVPs,
	}
}

// bootStartMsg is the synthetic message Init fires once on launch so the
// boot splash activates without mutating the constructor's value-typed
// Model. Tests that don't call Init never see this message — they bypass
// the splash entirely.
type bootStartMsg struct{}

// Init implements tea.Model. Starts the animation tick loop. In live
// mode, also fires the first snapshot immediately and starts the hook
// server watcher if a hook server was provided.
//
// Boot splash is opt-out via Config.BootScreenEnabled and only activates
// after Init runs (i.e. tea.NewProgram(...).Run()) so unit tests that
// build a Model and call View() directly never see it.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd(m.tickGen, activeTickInterval)}
	if m.cfg.BootScreenEnabled {
		cmds = append(cmds, func() tea.Msg { return bootStartMsg{} })
	}
	if !m.demoMode && m.liveSession != nil {
		cmds = append(cmds, m.snapshotCmd())
	}
	if m.hookServer != nil {
		cmds = append(cmds, hookWatchCmd(m.hookServer.Events()))
	}
	if c := m.planUsageCmd(); c != nil {
		// First fetch immediately on startup, then tick every 5 min.
		cmds = append(cmds, c, m.planUsageRefreshCmd())
	}
	return tea.Batch(cmds...)
}

// View implements tea.Model. Composes all panels.
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	// Boot splash takes the whole screen until it auto-finishes or the
	// user dismisses it. Rendered in the active theme so the user sees
	// the new palette on launch even before they hit the main UI.
	if m.boot.Active {
		v := tea.NewView(renderBootScreen(m.palette, m.boot, m.width, m.height))
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}

	var content string
	switch {
	case m.viewerActive && m.viewerFullscreen:
		// Fullscreen takeover: viewer occupies everything between the title
		// bar and the status bar. Clyde is meant to live in a side pane next
		// to claude, so the explorer + servers columns are precious — when
		// the user is reading a file, give it ALL the horizontal real estate
		// instead of forcing them to flip to a separate vscode window.
		content = m.renderFullscreenViewer()
	case m.effectiveMode() == LayoutMultiCol:
		content = m.renderMultiCol()
	case m.effectiveMode() == LayoutTabs:
		content = m.renderTabs()
	default:
		content = m.renderStack()
	}

	// Settings overlay: render on top via absolute cursor positioning.
	// The overlay string uses CSI sequences so it paints over the existing content
	// without disturbing Bubble Tea's normal line-count tracking.
	if m.settingsOpen {
		params := settingsViewParams{
			toggles:           m.settingsToggles,
			cursor:            m.settingsCursor,
			scope:             m.settingsScope,
			hasProjectScope:   m.liveProject.CWD() != "",
			cwd:               m.liveProject.CWD(),
			layoutMode:        m.layoutMode,
			rememberLayout:    m.settingsRememberLayout,
			notificationStyle: m.settingsNotificationStyle,
			costThreshold:     m.settingsCostThreshold,
			theme:             m.settingsTheme,
			mascotPersona:     m.settingsMascotPersona,
			bootScreenEnabled: m.settingsBootScreenEnabled,
		}
		titleBar := renderTitleBar(m.styles, m.palette, m.data, m.frame, m.width, m.demoMode, m.liveView, m.liveView.LastUpdate)
		statusBar := renderStatusBar(m.styles, m.width, m.isActiveMode(), m.copyToast, m.data.Sessions, m.helpOpen)
		content = renderSettingsFullScreen(m.styles, m.palette, params, titleBar, statusBar, m.width, m.height)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	// Mouse capture is always on. Native terminal drag-to-select in
	// alt-screen mode picks up everything visually present — line
	// numbers, panel borders, hint rows, neighboring panels — and the
	// terminal has no way to know which characters are "content" vs
	// "chrome". Letting it leak that into the user's clipboard is worse
	// than not offering mouse selection at all.
	//
	// Keyboard selection covers the same use case cleanly: shift+arrow
	// (with Ctrl/Alt/Cmd modifiers) extends a chrome-free range, ⌃c /
	// ⌘c copies via OSC 52. For long ranges that don't fit on screen,
	// :y N,M yanks line ranges by number.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
