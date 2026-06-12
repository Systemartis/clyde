package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/event"
)

// nowIdleThreshold is the duration after which a session with no new events is
// considered idle. The "idle" label replaces the last tool name.
const nowIdleThreshold = 30 * time.Second

// assistantFreshnessWindow defines how long after an assistant text or
// thinking event we still call the state "writing" / "thinking". Events
// in claude code's JSONL are committed atomically — once the file is
// written, claude is not actively producing more output for that turn.
// Past this cutoff we transition to "awaiting" so the panel reflects
// reality instead of pretending claude is still streaming a response
// that landed seconds ago.
const assistantFreshnessWindow = 5 * time.Second

// thinkingLookThreshold is the duration of a continuous thinking block that
// triggers the LookAround mascot state (bunny looks around as if curious).
const thinkingLookThreshold = 10 * time.Second

// newToolResultEvents returns all events after the event identified by prevID
// that are KindUser + IsToolResultOnly. Used to detect freshly completed tool
// calls since the last snapshot and trigger mascot reactions.
func newToolResultEvents(evts []event.Event, prevID string) []event.Event {
	if len(evts) == 0 {
		return nil
	}
	// Find the cut-off index: first event after prevID.
	startIdx := 0
	if prevID != "" {
		for i, ev := range evts {
			if ev.ID == prevID {
				startIdx = i + 1
				break
			}
		}
	}
	var out []event.Event
	for i := startIdx; i < len(evts); i++ {
		ev := evts[i]
		if ev.Kind != event.KindUser {
			continue
		}
		up, ok := ev.Payload.(event.UserPayload)
		if ok && up.IsToolResultOnly {
			out = append(out, ev)
		}
	}
	return out
}

// isLongThinking returns true when the most recent assistant event looks like an
// active thinking block that has been running for more than threshold.
// "Thinking" is detected by checking the last KindAssistant event for a
// "(thinking)" summary or a summary containing the word "thinking".
func isLongThinking(evts []event.Event, now time.Time, threshold time.Duration) bool {
	for i := len(evts) - 1; i >= 0; i-- {
		ev := evts[i]
		if ev.Kind != event.KindAssistant {
			continue
		}
		ap, ok := ev.Payload.(event.AssistantPayload)
		if !ok {
			break
		}
		sum := strings.ToLower(ap.Summary)
		isThinking := strings.Contains(sum, "thinking") || ap.Summary == "(thinking)"
		if !isThinking {
			break
		}
		elapsed := now.Sub(ev.Timestamp)
		if elapsed < 0 {
			elapsed = 0
		}
		return elapsed > threshold
	}
	return false
}

// NowStatus holds the derived display strings for the now panel.
// It is produced by deriveNowStatus and consumed by applyLiveView.
type NowStatus struct {
	// Op is the concise action string shown on line 1:
	// "edit auth.ts", "read main.go", "bash 'go test'", "thinking…", etc.
	Op string

	// Meta is the secondary detail string shown on line 2:
	// elapsed time for bash, token rate for thinking, path tail for reads.
	Meta string

	// ModeText is the header badge text (top-right of panel border):
	// "writing · NN t/s" during streaming, tool name or "idle" otherwise.
	ModeText string

	// MascotTrigger is the mascot event to fire on this status update.
	// Valid trigger values are eventHappy, eventSurprised, eventSleep.
	MascotTrigger mascotEventKind

	// HasTrigger is true when MascotTrigger should be applied.
	// Separates "no trigger" from eventBlink (index 0).
	HasTrigger bool
}

// deriveNowStatus builds the NowStatus for the now panel from the most recent
// visible event in the LiveSession view.
//
// Priority chain:
//  1. Last assistant tool_use → verb + key arg.
//  2. Last assistant text turn → "thinking…" or "writing response".
//  3. Last user event → "awaiting".
//  4. No recent events (>30s) → "idle".
//  5. Default fallback → "session active".
func deriveNowStatus(v livesession.View) NowStatus {
	now := v.LastUpdate
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if len(v.Events) == 0 {
		return NowStatus{Op: "waiting", ModeText: "idle"}
	}

	last := v.Events[len(v.Events)-1]
	age := now.Sub(last.Timestamp)
	if age < 0 {
		age = 0
	}

	// Idle check: no activity for more than the threshold.
	// Meta counts UP from the moment idle began (age - threshold) instead
	// of echoing raw event age — when the panel transitions from
	// "writing" / "awaiting" to "idle" the user expects a fresh
	// state-relative counter, not the same number with a different label.
	if age > nowIdleThreshold {
		idleFor := age - nowIdleThreshold
		meta := ""
		if idleFor >= time.Second {
			meta = "for " + formatDuration(idleFor)
		}
		return NowStatus{
			Op:            "idle",
			Meta:          meta,
			ModeText:      "idle",
			MascotTrigger: eventSleep,
			HasTrigger:    true,
		}
	}

	// Tool call: most recent assistant event with a tool summary.
	if st, ok := lastToolCallStatus(v.Events, now); ok {
		return st
	}

	// Assistant text turn: thinking or writing.
	if st, ok := lastAssistantTextStatus(v.Events, now); ok {
		return st
	}

	// User event: awaiting.
	return lastUserEventStatus(v.Events)
}

// lastToolCallStatus reports a "running" tool status ONLY when the very
// latest event is an assistant tool_use. If anything came after it
// (tool_result, user reply, follow-up assistant message) the call has ended
// and we should fall through to text / awaiting status — the prior version
// would show "running ~ 1m" indefinitely on stale tool_use events while
// claude had long since moved on.
func lastToolCallStatus(evts []event.Event, now time.Time) (NowStatus, bool) {
	if len(evts) == 0 {
		return NowStatus{}, false
	}
	last := evts[len(evts)-1]
	if last.Kind != event.KindAssistant {
		return NowStatus{}, false
	}
	ap, ok := last.Payload.(event.AssistantPayload)
	if !ok || !strings.HasPrefix(ap.Summary, "Tool: ") {
		return NowStatus{}, false
	}
	tool, arg := parseSummary(ap.Summary)
	op := toolVerb(tool, arg)
	elapsed := now.Sub(last.Timestamp)
	if elapsed < 0 {
		elapsed = 0
	}
	meta := "running ~ " + formatDuration(elapsed)
	modeText := strings.ToLower(tool)
	return NowStatus{Op: op, Meta: meta, ModeText: modeText}, true
}

// lastAssistantTextStatus finds the most recent assistant text-only
// turn and reports the right derived status:
//
//   - within assistantFreshnessWindow → "writing" / "thinking" (we
//     assume the response is still streaming or just landed)
//   - past the freshness cutoff but within the idle threshold →
//     "awaiting" (the response committed; claude is waiting for the
//     user, the session is not yet idle)
//
// Without the freshness gate the panel happily showed "writing
// response 22s" long after claude had handed control back to the user,
// which left the now panel feeling stuck.
func lastAssistantTextStatus(evts []event.Event, now time.Time) (NowStatus, bool) {
	for i := len(evts) - 1; i >= 0; i-- {
		ev := evts[i]
		if ev.Kind != event.KindAssistant {
			continue
		}
		ap, ok := ev.Payload.(event.AssistantPayload)
		if !ok {
			continue
		}
		if strings.HasPrefix(ap.Summary, "Tool: ") {
			// Tool call — handled above.
			break
		}
		if ap.Summary == "" {
			continue
		}

		elapsed := now.Sub(ev.Timestamp)
		if elapsed < 0 {
			elapsed = 0
		}
		isThinking := strings.Contains(strings.ToLower(ap.Summary), "thinking") || ap.Summary == "(thinking)"

		// Past the freshness cutoff: response is definitely committed.
		// Show an "awaiting" state so the user knows claude has handed
		// control back. Meta carries the time-since-response so the
		// counter still reads as informative.
		if elapsed >= assistantFreshnessWindow {
			op := "awaiting reply"
			modeText := "responded"
			if isThinking {
				op = "awaiting"
				modeText = "thought"
			}
			return NowStatus{
				Op:       op,
				Meta:     formatAge(elapsed),
				ModeText: modeText,
			}, true
		}

		// Fresh — likely still streaming.
		tps := tokenRate(int(ap.Usage.Output), elapsed)
		elapsedStr := formatDuration(elapsed)
		if isThinking {
			modeText := "thinking"
			if tps > 0 {
				modeText = fmt.Sprintf("thinking · %d t/s", tps)
			}
			return NowStatus{
				Op:       "thinking…",
				Meta:     elapsedStr,
				ModeText: modeText,
			}, true
		}

		modeText := "writing response"
		if tps > 0 {
			modeText = fmt.Sprintf("writing · %d t/s", tps)
		}
		return NowStatus{
			Op:       "writing response",
			Meta:     elapsedStr,
			ModeText: modeText,
		}, true
	}
	return NowStatus{}, false
}

// lastUserEventStatus produces a NowStatus from the last user event.
func lastUserEventStatus(evts []event.Event) NowStatus {
	for i := len(evts) - 1; i >= 0; i-- {
		ev := evts[i]
		if ev.Kind != event.KindUser {
			continue
		}
		return NowStatus{
			Op:       "awaiting",
			Meta:     formatTimestamp(ev.Timestamp),
			ModeText: "queued",
		}
	}
	return NowStatus{Op: "session active", ModeText: "active"}
}

// toolVerb maps a tool name and its key arg to a concise "verb arg" string.
// The verb is chosen per the Phase F spec:
//
//	Edit/MultiEdit/Write → "edit <filename only>"
//	Read                 → "read <filename>"
//	Bash                 → "bash '<cmd>'"
//	Grep                 → "grep <pattern>"
//	Task                 → "dispatch <subagent_type>"
//	default              → "<tool> <key-arg>"
func toolVerb(tool, arg string) string {
	switch strings.ToLower(tool) {
	case "edit", "multiedit", "write":
		if arg != "" {
			return truncate("edit "+filepath.Base(arg), 50)
		}
		return "edit"
	case "read":
		if arg != "" {
			return truncate("read "+filepath.Base(arg), 50)
		}
		return "read"
	case "bash":
		if arg != "" {
			cmd := strings.Trim(arg, "'\" ")
			return truncate("bash '"+cmd+"'", 50)
		}
		return "bash"
	case "grep":
		if arg != "" {
			return truncate("grep "+arg, 50)
		}
		return "grep"
	case "task":
		if arg != "" {
			return truncate("dispatch "+arg, 50)
		}
		return "dispatch"
	default:
		verb := strings.ToLower(tool)
		if arg != "" {
			return truncate(verb+" "+arg, 50)
		}
		return verb
	}
}

// tokenRate computes tokens-per-second from a token count and elapsed duration.
// Returns 0 when elapsed is too small to produce a meaningful rate.
func tokenRate(tokens int, elapsed time.Duration) int {
	if elapsed < 500*time.Millisecond || tokens <= 0 {
		return 0
	}
	return int(float64(tokens) / elapsed.Seconds())
}
