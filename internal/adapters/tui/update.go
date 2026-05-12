package tui

import (
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	"github.com/Systemartis/clyde/internal/adapters/hookserver"
	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/event"
)

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg), nil

	case FrameMsg:
		return m.handleFrame(msg)

	case bootStartMsg:
		// Init() fires this once when BootScreenEnabled. Activate the
		// splash; the FrameMsg loop already running will keep advancing
		// it until auto-dismiss or a user keypress.
		m.boot.Active = true
		m.boot.Tick = 0
		return m, nil

	case progress.FrameMsg:
		// Forward progress FrameMsg to each progress bar so they animate.
		var cmds []tea.Cmd
		var c tea.Cmd
		m.progTokens, c = m.progTokens.Update(msg)
		cmds = append(cmds, c)
		m.progReset, c = m.progReset.Update(msg)
		cmds = append(cmds, c)
		return m, tea.Batch(cmds...)

	case liveSessionMsg:
		return m.handleLiveSession(msg)

	case refreshLiveMsg:
		return m.handleRefreshLive()

	case planUsageMsg:
		return m.handlePlanUsage(msg)

	case refreshPlanUsageMsg:
		return m, m.planUsageCmd()

	case hookEventMsg:
		return m.handleHookEvent(msg)

	case tea.KeyPressMsg:
		wake := m.markInteraction()
		next, cmd := m.handleKey(msg)
		return next, tea.Batch(cmd, wake)

	case tea.MouseClickMsg:
		wake := m.markInteraction()
		next, cmd := m.handleMouseClick(msg)
		return next, tea.Batch(cmd, wake)

	case tea.MouseWheelMsg:
		wake := m.markInteraction()
		next, cmd := m.handleMouseWheel(msg)
		return next, tea.Batch(cmd, wake)

	case tea.PasteMsg:
		// Bracketed paste — system Cmd+V / Ctrl+V drops the entire
		// pasted block here as one event. Only consumed by the editor
		// (insert at cursor); other modes ignore it so a stray paste
		// outside the viewer doesn't go anywhere unexpected.
		if m.viewerActive && m.viewerMode == ViewerEdit && msg.Content != "" {
			for _, r := range msg.Content {
				if r == '\n' {
					m.viewerEdit = insertNewline(m.viewerEdit)
					continue
				}
				if r == '\r' {
					continue
				}
				if r == '\t' {
					for range viewerTabWidth {
						m.viewerEdit = insertRune(m.viewerEdit, ' ')
					}
					continue
				}
				m.viewerEdit = insertRune(m.viewerEdit, r)
			}
			m.viewerDirty = true
		}
		return m, nil

	case clearCopyToastMsg:
		// Older toast tick fired but a fresh yank moved the expiry into
		// the future — leave the new toast alone, the next tick will pick it up.
		if !m.copyToastExpires.IsZero() && time.Now().Before(m.copyToastExpires) {
			return m, nil
		}
		m.copyToast = ""
		m.copyToastExpires = time.Time{}
		return m, nil
	}
	return m, nil
}

// handleWindowSize applies a resize event: stores dimensions, recalculates
// the expanded heights for all collapse springs, and syncs viewport widths so
// scrollable panel content is re-laid out at the correct inner width.
//
// Per-panel manual heights in m.panelHeights take precedence over the
// computed default so a RememberLayout-restored layout survives the
// startup WindowSizeMsg and subsequent terminal resizes. Each height is
// clamped to the terminal's available rows.
func (m Model) handleWindowSize(msg tea.WindowSizeMsg) Model {
	m.width = msg.Width
	m.height = msg.Height
	m.bp = DetectBreakpoint(msg.Width)
	maxH := float64(m.height - 8)
	if maxH < float64(panelHeightMin) {
		maxH = float64(panelHeightMin)
	}
	for i := PanelID(0); i < panelCount; i++ {
		var h float64
		if persisted := m.panelHeights[i]; persisted > 0 {
			h = float64(persisted)
		} else {
			h = m.defaultExpandedHeight(i, m.height-9, m.bp)
		}
		if h > maxH {
			h = maxH
		}
		if h < float64(panelHeightMin) {
			h = float64(panelHeightMin)
		}
		m.collapse[i].SetExpandedHeight(h)
		// Resync each viewport so its stored content width matches the new panel width.
		// This prevents stale-width content from clipping after a terminal resize.
		// syncPanelViewport is a no-op for panels without scrollable viewports.
		m = m.syncPanelViewport(i)
	}
	return m
}

// handleFrame advances the animation tick, all collapse springs, and the
// notification lerp, then schedules the next tick at the cadence picked
// by nextTickInterval (fast while animating, slow when idle).
//
// FrameMsgs whose Gen does not match the current tickGen are stale ticks
// from a superseded chain (e.g. a slow tick that was already in flight
// when the user pressed a key and switched us back to fast mode). Drop
// them — the new chain has already scheduled its own next tick.
func (m Model) handleFrame(msg FrameMsg) (tea.Model, tea.Cmd) {
	if msg.Gen != m.tickGen {
		return m, nil
	}
	// Sync the mascot's "claude is working" hint from the now-panel mode
	// text BEFORE advancing — the picker reads .working when it schedules
	// the next idle event, so a stale flag would let an attentive look
	// fire a half-second after claude went quiet.
	m.frame.Mascot = m.frame.Mascot.WithWorking(m.isClaudeWorking())
	m.frame = AdvanceTick(m.frame)
	if m.boot.Active {
		m.boot = m.boot.Advance()
	}
	for i := range m.collapse {
		m.collapse[i].Advance()
	}
	if m.notifPos < 1.0 {
		// Easing coefficient sized for the 50ms tick interval so the
		// slide-in feels the same at any tick rate. Halve when the
		// tick rate doubles.
		m.notifPos += (1.0 - m.notifPos) * 0.2
		if m.notifPos > 0.99 {
			m.notifPos = 1.0
		}
	}
	m.tickGen++
	return m, tickCmd(m.tickGen, m.nextTickInterval())
}

// sessionIDsKnown returns the set of session IDs visible in a snapshot
// for set-difference checks (was-this-session-here-last-tick?). The
// answer drives the auto-switch-to-Σ behavior when the user opens a
// fresh claude code session in the same cwd.
func sessionIDsKnown(stats []livesession.SessionStat) map[string]struct{} {
	out := make(map[string]struct{}, len(stats))
	for _, st := range stats {
		out[string(st.ID)] = struct{}{}
	}
	return out
}

// hasNewSession reports whether any ID in current is not in prev.
// True ⇒ a session appeared this tick.
func hasNewSession(prev, current map[string]struct{}) bool {
	for id := range current {
		if _, was := prev[id]; !was {
			return true
		}
	}
	return false
}

// loadViewerDiff fetches the diff hunks for the file currently open in the
// viewer and stores them on the model so the viewer can paint green/red
// highlights for any modified file the user opens — not just the focused
// session's claude-edited file.
//
// Demo mode and missing diffSrc both fall back to clearing the per-viewer
// hunks; the viewer renderer treats nil viewerDiffLines as "no overlay"
// and uses the global m.data.DiffLines if it happens to match the path
// (existing behavior preserved for the focused-session flow).
func (m Model) loadViewerDiff() Model {
	m.viewerDiffFile = ""
	m.viewerDiffLines = nil
	if m.demoMode || m.diffSrc == nil || m.viewerFile == "" {
		return m
	}
	cwd := m.liveView.Project.CWD()
	hunks, err := m.diffSrc.Diff(cwd, m.viewerFile)
	if err != nil || len(hunks) == 0 {
		return m
	}
	m.viewerDiffFile = m.viewerFile
	m.viewerDiffLines = convertDiffHunks(hunks)
	return m
}

// applyLiveView copies data from liveView into the MockData fields used by panels
// that are wired in Phases A–G. Panels not yet wired keep their existing MockData
// values unchanged.
func (m Model) applyLiveView() Model {
	v := m.liveView

	// ── Title bar ────────────────────────────────────────────────────────────
	if v.Project.CWD() != "" {
		dirPath, projName := deriveTitlePath(v.Project.CWD())
		m.data.ProjectPath = dirPath
		m.data.ProjectName = projName
	}

	// ── Calls panel ──────────────────────────────────────────────────────────
	// Always overwrite in live mode — empty IS valid live state, especially
	// after the user switches to a fresh session via the tab strip. Keeping
	// the previous session's groups was a mock-fallback hangover that made
	// activity look stale across switches.
	m.data.AgentGroups = deriveAgentGroups(v)

	// ── Now panel ────────────────────────────────────────────────────────────
	// Same reasoning as Calls — clear when the new session has nothing to
	// report instead of leaking the prior session's "now" line.
	ns := deriveNowStatus(v)
	m.data.NowOp = ns.Op
	m.data.NowMeta = ns.Meta
	m.data.NowMode = ns.ModeText

	// ── Usage panel ──────────────────────────────────────────────────────────
	m.data = deriveUsageFields(v, m.data)
	// Overlay real plan-quota numbers (Anthropic /api/oauth/usage) when the
	// adapter is wired and a fetch has succeeded. Otherwise the JSONL
	// time-elapsed approximation from deriveUsageFields stays in place.
	// v.LastUpdate is the Clock instant of this snapshot, so the reset
	// countdown ticks every refresh instead of freezing until the next
	// 5-minute plan-usage fetch.
	m.data = applyPlanUsageToMock(m.data, m.planUsage, m.planUsageErr, v.LastUpdate)

	// ── Explorer panel (Phase D) ─────────────────────────────────────────────
	m.data = deriveExplorerData(v, m.data)
	// Rebuild visible rows from fresh tree data.
	m.explorer.RefreshRows(m.data)

	// ── Servers panel (Phase G) ───────────────────────────────────────────────
	m.data = deriveServersFields(v, m.data)

	// ── Bash audit panel (Phase 5) ────────────────────────────────────────────
	m.data = deriveBashLog(v, m.data)

	// ── Cache efficiency panel (Phase 6) ──────────────────────────────────────
	// On the Σ aggregate tab, show stats merged across every recent session
	// in the cwd so the user gets a meaningful "how is caching doing across
	// my work in this folder?" reading instead of single-session noise.
	m.data = deriveCacheStats(v, m.data, m.sessionTabIndex == -1)

	// ── Multi-session tab strip ───────────────────────────────────────────────
	now := v.LastUpdate
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tabs := sessionTabsFromView(v, m.sessionTabIndex, now)
	m.data.Sessions = tabs
	if len(tabs) == 0 {
		// No tab strip on screen → keep the user's choice but treat -1
		// (Σ) as default-focus so the usage panel stays in single-bar mode.
		m.data.SessionTabIndex = m.sessionTabIndex
		m.data.SessionLeaderboard = nil
	} else {
		m.data.SessionTabIndex = m.sessionTabIndex
		// Σ tab → render the leaderboard; specific tab → single bar.
		if m.sessionTabIndex == -1 {
			m.data.SessionLeaderboard = sessionLeaderboardFromView(v, now)
		} else {
			m.data.SessionLeaderboard = nil
		}
	}

	// ── Diff panel (Phase E) ──────────────────────────────────────────────────
	// DiffHunks were fetched in snapshotCmd (goroutine-safe). Convert to the
	// mock DiffLine format that panel_diff.go renders.
	// Always update in live mode (even empty hunks replaces stale mock data).
	if !m.demoMode {
		m.data.DiffFile = deriveDiffLabel(v)
		m.data.DiffLines = convertDiffHunks(v.DiffHunks)
		m.data.IsGitRepo = m.liveIsGitRepo
	}

	// ── Compaction state (Phase I) ────────────────────────────────────────────
	m.compaction = deriveCompactionState(m.data.TokenPct)

	// ── Active-panel viewport resync ──────────────────────────────────────────
	// Active mode pipes the panel through its viewport, whose content was
	// captured at activation time. Rebuild it from the fresh snapshot so the
	// stream keeps flowing while the user scrolls (SetContent preserves the
	// scroll offset, clamping only when content shrank).
	if m.isActiveMode() {
		m = m.syncPanelViewport(m.activePanelID)
	}

	// ── Quota / cost thresholds ───────────────────────────────────────────────
	// Re-evaluate every snapshot — session cost grows with usage events
	// rather than plan-usage polls, so the cost-threshold latch needs
	// per-snapshot resolution. Plan-quota latches are cheap to re-check
	// on every tick; the latch map suppresses re-fires.
	m = m.evaluateQuotaThresholds()

	return m
}

// handleLiveSession processes a completed LiveSession snapshot.
// Errors are swallowed — the UI shows empty/stale state rather than crashing.
// On each snapshot, it:
//  1. Checks whether the snapshot differs from the previous one (I.5 debounce).
//  2. If changed: replaces liveView and applies derived data to MockData fields.
//  3. Compares the new latest event ID with the previous one to detect changes.
//  4. Triggers mascot reactions based on the kind of change detected.
func (m Model) handleLiveSession(msg liveSessionMsg) (tea.Model, tea.Cmd) {
	if msg.err == nil {
		// Always update git-repo status so the diff panel reflects the current cwd
		// even when the session content hasn't changed.
		m.liveIsGitRepo = msg.isGitRepo

		newView := msg.view
		newLatestID := latestEventID(newView.Events)
		newCount := len(newView.Events)

		// Auto-switch to Σ tab when a brand-new session appears in this
		// cwd — the user just opened a fresh claude code, so the most
		// useful view is the cross-session aggregate. Opt out by setting
		// auto_switch_to_all_on_new_session = false in clyde config.
		prevSessionIDs := sessionIDsKnown(m.liveView.SessionStats)
		newSessionIDs := sessionIDsKnown(newView.SessionStats)
		if hasNewSession(prevSessionIDs, newSessionIDs) && m.cfg.AutoSwitchToAllOnNewSession {
			m.sessionTabIndex = -1
		}

		// Always re-apply the live view. Tab visibility depends on
		// `now` (a session that's been silent for > activeSessionWindow
		// must drop out instantly when the cursor crosses that boundary),
		// and the cost of re-deriving everything per second is small. The
		// older debounce traded correctness for micro-perf — closed
		// sessions used to cling to the tab strip until something else
		// kicked applyLiveView.
		if m.llmSource != "" {
			newView.LLMSource = m.llmSource
		}
		m.liveView = newView
		m = m.applyLiveView()

		// ── Mascot event triggers ─────────────────────────────────────────────
		// Only trigger when a genuinely new event arrived (ID changed).
		if newLatestID != "" && newLatestID != m.prevLatestEventID {
			m.frame.Mascot = m.mascotTriggerForNewEvent(newView.Events)
		}
		m.prevLatestEventID = newLatestID
		m.prevEventCount = newCount
	}
	return m, m.refreshCmd()
}

// latestEventID returns the ID of the last event in the slice, or "" if empty.
func latestEventID(evts []event.Event) string {
	if len(evts) == 0 {
		return ""
	}
	return evts[len(evts)-1].ID
}

// mascotTriggerForNewEvent inspects the newest event and returns an updated
// MascotFSM with an appropriate external state queued.
//
// Rules (Phase I):
//   - Age of latest event > nowIdleThreshold → Sleep.
//   - New tool_result event with is_error:true → Surprised (2 s ≈ 4 hold frames @ 8 each).
//   - New tool_result event without error → Happy (3 s ≈ 8 hold frames, matches happySequence).
//   - Thinking block running > thinkingLookThreshold → LookAround (curious idle).
//   - Otherwise → no trigger; FSM advances autonomously.
func (m Model) mascotTriggerForNewEvent(evts []event.Event) MascotFSM {
	fsm := m.frame.Mascot

	if len(evts) == 0 {
		return fsm
	}

	latestEvt := evts[len(evts)-1]

	// Check for idle: trigger Sleep when no new activity in threshold.
	now := m.liveView.LastUpdate
	if now.IsZero() {
		now = time.Now().UTC()
	}
	age := now.Sub(latestEvt.Timestamp)
	if age > nowIdleThreshold {
		return fsm.SetExternalState(eventSleep)
	}

	// Detect new tool_result events (KindUser with IsToolResultOnly).
	// Scan from the previously-seen event boundary to find new ones.
	newToolResults := newToolResultEvents(evts, m.prevLatestEventID)
	for _, ev := range newToolResults {
		up, ok := ev.Payload.(event.UserPayload)
		if !ok || !up.IsToolResultOnly {
			continue
		}
		if up.ToolResultError {
			// Error in tool_result → Surprised
			return fsm.SetExternalState(eventSurprised)
		}
		// Successful tool_result → Happy
		return fsm.SetExternalState(eventHappy)
	}

	// Detect long thinking blocks → LookAround (curious).
	if isLongThinking(evts, now, thinkingLookThreshold) {
		return fsm.SetExternalState(eventLookAround)
	}

	// No trigger — autonomous FSM handles blink/look/happy.
	return fsm
}

// handleRefreshLive fires the next snapshot when in live mode.
func (m Model) handleRefreshLive() (tea.Model, tea.Cmd) {
	if !m.demoMode && m.liveSession != nil {
		return m, m.snapshotCmd()
	}
	return m, nil
}

// handlePlanUsage processes the result of a Fetch and reschedules the next
// tick. On success the new PlanUsage replaces the cached value; on failure
// we keep the previous value (graceful degradation) and store the error so
// the panel can surface a "(plan offline)" badge.
//
// After applying the new numbers we re-evaluate quota thresholds so a
// fresh 5h/weekly crossing fires its notification immediately rather
// than waiting for the next live snapshot.
func (m Model) handlePlanUsage(msg planUsageMsg) (tea.Model, tea.Cmd) {
	m.planUsageErr = msg.err
	if msg.err == nil {
		m.planUsage = msg.usage
	}
	// Re-merge the latest live view so the usage panel picks up the new percentages.
	if !m.demoMode && m.liveSession != nil {
		m = m.applyLiveView()
	} else {
		// Demo mode: still propagate to mock data so the bars reflect the
		// (mock) plan-usage values configured in V3MockData.
		m.data = applyPlanUsageToMock(m.data, m.planUsage, m.planUsageErr, time.Now().UTC())
	}
	m = m.evaluateQuotaThresholds()
	return m, m.planUsageRefreshCmd()
}

// handleHookEvent processes an incoming hook event from the hook server.
// It populates hookNotif so the notification banner shows the live event,
// stores the ResponseCh for later y/n response, and immediately queues the
// next watch so subsequent events are not dropped.
func (m Model) handleHookEvent(msg hookEventMsg) (tea.Model, tea.Cmd) {
	evt := msg.evt

	keyArg := extractKeyArg(evt.Tool, evt.Args)

	m.hookNotif = HookNotification{
		Active: true,
		Tool:   evt.Tool,
		KeyArg: keyArg,
		Cwd:    evt.Cwd,
	}
	m.hookPendingCh = evt.ResponseCh

	// Clear any previous Esc-dismiss since a new event arrived.
	m.notifAck = false

	// Keep watching for the next event.
	var nextWatch tea.Cmd
	if m.hookServer != nil {
		nextWatch = hookWatchCmd(m.hookServer.Events())
	}
	return m, nextWatch
}

// respondHook sends resp to the pending hook's ResponseCh and clears hookNotif.
// No-op when no hook event is pending.
func (m *Model) respondHook(allow bool, reason string) {
	if m.hookPendingCh == nil {
		return
	}
	m.hookPendingCh <- hookserver.HookResponse{Allow: allow, Reason: reason}
	m.hookPendingCh = nil
	m.hookNotif = HookNotification{}
}

// extractKeyArg derives a concise key argument string from a tool's Args map.
// For Bash → "command"; for Edit/Write → "file_path"; for Read → "file_path".
// Falls back to the first string value found, or empty string.
func extractKeyArg(tool string, args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	var preferredKeys []string
	switch tool {
	case "Bash":
		preferredKeys = []string{"command"}
	case "Edit", "MultiEdit", "Write":
		preferredKeys = []string{"file_path", "path"}
	case "Read":
		preferredKeys = []string{"file_path", "path"}
	default:
		preferredKeys = []string{"file_path", "path", "command", "query"}
	}
	for _, k := range preferredKeys {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	// Fallback: first string value.
	for _, v := range args {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}
