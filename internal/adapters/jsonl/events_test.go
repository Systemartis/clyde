package jsonl_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clyde-tui/clyde/internal/adapters/jsonl"
	"github.com/clyde-tui/clyde/internal/domain/event"
	"github.com/clyde-tui/clyde/internal/domain/session"
)

// buildAssistantLineWithThinking returns a JSONL line representing an assistant
// event whose thinking block exceeds the given size in bytes.
// This is generated programmatically so no large file lives in the repo.
func buildAssistantLineWithThinking(thinkingSize int) string {
	thinking := strings.Repeat("x", thinkingSize)
	return fmt.Sprintf(
		`{"type":"assistant","uuid":"large-thinking-uuid","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"large-session","parentUuid":"parent-uuid","message":{"model":"claude-opus-4-7","id":"msg_large","type":"message","role":"assistant","content":[{"type":"thinking","thinking":%q}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		thinking,
	)
}

// TestEvents_SimpleUserAssistant reads the simple-user-assistant.jsonl fixture
// and asserts:
//   - exactly 2 events are returned
//   - first event is KindUser
//   - second event is KindAssistant with non-zero Usage
//   - events are ordered chronologically (ascending timestamps)
func TestEvents_SimpleUserAssistant(t *testing.T) {
	t.Parallel()

	// Build a Source that resolves relative to our testdata directory.
	// We use a fake base dir that maps a project cwd to the fixture.
	// The fixture is at testdata/simple-user-assistant/<sessionID>.jsonl.
	// We construct the Source with that base and pass a matching cwd + session ID.
	const sessionID = "simple-session"
	dir := t.TempDir()

	// Encoded project path: our fake cwd "/test/project" → "-test-project"
	encoded := "-test-project"
	projectDir := filepath.Join(dir, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Read the fixture and copy into the temp dir.
	fixtureData, err := os.ReadFile("testdata/simple-user-assistant.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), fixtureData, 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Events() returned %d events; want 2", len(events))
	}

	if events[0].Kind != event.KindUser {
		t.Errorf("events[0].Kind = %q; want %q", events[0].Kind, event.KindUser)
	}
	if events[1].Kind != event.KindAssistant {
		t.Errorf("events[1].Kind = %q; want %q", events[1].Kind, event.KindAssistant)
	}

	// Assert chronological order.
	if !events[0].Timestamp.Before(events[1].Timestamp) {
		t.Errorf("events are not in chronological order: %v >= %v",
			events[0].Timestamp, events[1].Timestamp)
	}

	// Assert assistant payload carries usage.
	ap, ok := events[1].Payload.(event.AssistantPayload)
	if !ok {
		t.Fatalf("events[1].Payload is %T; want event.AssistantPayload", events[1].Payload)
	}
	if ap.Usage.Output == 0 {
		t.Errorf("AssistantPayload.Usage.Output = 0; want non-zero")
	}
}

// TestEvents_LargeThinkingBlock asserts that an assistant event with a thinking
// block exceeding the default 64KB bufio.Scanner token limit does NOT cause a
// "token too long" error. The fixture is generated programmatically (no large
// static file in the repo).
func TestEvents_LargeThinkingBlock(t *testing.T) {
	t.Parallel()

	const sessionID = "large-session"
	dir := t.TempDir()

	// Encoded project path for fake cwd "/test/large" → "-test-large"
	encoded := "-test-large"
	projectDir := filepath.Join(dir, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 80KB thinking block — well above the default 64KB Scanner limit.
	line := buildAssistantLineWithThinking(80 * 1024)
	content := line + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() returned error for large thinking block: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Events() returned %d events; want 1", len(events))
	}
	if events[0].Kind != event.KindAssistant {
		t.Errorf("events[0].Kind = %q; want %q", events[0].Kind, event.KindAssistant)
	}
}

// TestEvents_UnknownTypes reads the unknown-types.jsonl fixture and asserts
// that events with unrecognized type strings are returned as OpaquePayload
// with the raw line bytes preserved.
func TestEvents_UnknownTypes(t *testing.T) {
	t.Parallel()

	const sessionID = "unknown-session"
	dir := t.TempDir()

	encoded := "-test-unknown"
	projectDir := filepath.Join(dir, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fixtureData, err := os.ReadFile("testdata/unknown-types.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), fixtureData, 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("Events() returned 0 events; want at least 1")
	}

	// All events in the fixture are unknown types — each must be OpaquePayload.
	for i, ev := range events {
		if ev.Kind != event.KindOpaque {
			t.Errorf("events[%d].Kind = %q; want %q", i, ev.Kind, event.KindOpaque)
		}
		op, ok := ev.Payload.(event.OpaquePayload)
		if !ok {
			t.Errorf("events[%d].Payload is %T; want OpaquePayload", i, ev.Payload)
			continue
		}
		if len(op.Raw) == 0 {
			t.Errorf("events[%d].OpaquePayload.Raw is empty; want preserved raw bytes", i)
		}
	}
}

// TestEvents_MetaTypesWithoutEnvelope asserts that meta event types (last-prompt,
// ai-title, permission-mode, file-history-snapshot, etc.) which lack the standard
// envelope fields (uuid, timestamp, parentUuid) are PRESERVED as KindOpaque rather
// than erroring out. Only `type` is truly required for dispatch.
//
// Real evidence: line 1 of any active Claude Code session JSONL is a
// "last-prompt" entry of the shape:
//
//	{"type":"last-prompt","leafUuid":"<uuid>","sessionId":"<id>"}
//
// — no uuid, no timestamp, no parentUuid. The parser must NOT treat this as a
// fatal error; per spec, unknown kinds are preserved.
func TestEvents_MetaTypesWithoutEnvelope(t *testing.T) {
	t.Parallel()

	const sessionID = "meta-session"
	dir := t.TempDir()

	encoded := "-test-meta"
	projectDir := filepath.Join(dir, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Three lines:
	//   1. last-prompt — no uuid, no timestamp, no parentUuid (real shape)
	//   2. ai-title    — no uuid, no timestamp (real shape)
	//   3. user        — full envelope (regular event)
	lastPrompt := `{"type":"last-prompt","leafUuid":"5408aed2-6a65-4463-a370-4c8a79b607cb","sessionId":"meta-session"}`
	aiTitle := `{"type":"ai-title","title":"Some chat title","sessionId":"meta-session"}`
	regular := `{"type":"user","uuid":"u1","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"meta-session","parentUuid":""}`
	content := lastPrompt + "\n" + aiTitle + "\n" + regular + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() error on meta-types fixture: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("Events() returned %d events; want 3", len(events))
	}

	// First two are meta types — must be Opaque with raw line preserved.
	for i := 0; i < 2; i++ {
		if events[i].Kind != event.KindOpaque {
			t.Errorf("events[%d].Kind = %q; want %q (meta type should be opaque)",
				i, events[i].Kind, event.KindOpaque)
		}
		op, ok := events[i].Payload.(event.OpaquePayload)
		if !ok {
			t.Errorf("events[%d].Payload is %T; want OpaquePayload", i, events[i].Payload)
			continue
		}
		if len(op.Raw) == 0 {
			t.Errorf("events[%d].OpaquePayload.Raw is empty; want preserved raw bytes", i)
		}
	}

	// Third is a regular user event with full envelope.
	if events[2].Kind != event.KindUser {
		t.Errorf("events[2].Kind = %q; want %q", events[2].Kind, event.KindUser)
	}
	if events[2].ID != "u1" {
		t.Errorf("events[2].ID = %q; want %q", events[2].ID, "u1")
	}
}

// TestEvents_MissingFile asserts that Events() returns an error (not a panic)
// when the JSONL file does not exist.
func TestEvents_MissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := jsonl.NewSource(dir)

	// Session ID that has no file.
	_, err := src.Events(context.Background(), session.ID("nonexistent-session"))
	if err == nil {
		t.Error("Events() returned nil error for missing file; want error")
	}
}

// TestEvents_SessionIDConnectsToFile asserts that Events() can locate the file
// when the session ID matches the filename (without .jsonl extension) in the
// encoded project directory.
//
// This is an integration-level sanity check that the Source uses os.UserHomeDir
// + ~/.claude/projects/<encoded>/<sessionID>.jsonl path logic.
// We stub this by pointing NewSource at a temp dir that mimics the structure.
func TestEvents_IDsPopulated(t *testing.T) {
	t.Parallel()

	const sessionID = "id-check-session"
	dir := t.TempDir()

	// Minimal two-line JSONL.
	line1 := `{"type":"user","uuid":"user-uuid-1","timestamp":"2026-04-30T08:00:00.000Z","sessionId":"id-check-session","parentUuid":""}`
	line2 := `{"type":"assistant","uuid":"asst-uuid-2","timestamp":"2026-04-30T08:01:00.000Z","sessionId":"id-check-session","parentUuid":"user-uuid-1","message":{"model":"claude","id":"msg_1","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`

	encoded := "-test-ids"
	projectDir := filepath.Join(dir, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := line1 + "\n" + line2 + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("Events() returned %d events; want 2", len(events))
	}

	if events[0].ID != "user-uuid-1" {
		t.Errorf("events[0].ID = %q; want %q", events[0].ID, "user-uuid-1")
	}
	if events[0].SessionID != "id-check-session" {
		t.Errorf("events[0].SessionID = %q; want %q", events[0].SessionID, "id-check-session")
	}
	if events[1].ParentID != "user-uuid-1" {
		t.Errorf("events[1].ParentID = %q; want %q", events[1].ParentID, "user-uuid-1")
	}

	// Timestamps must be parsed correctly.
	wantTS, _ := time.Parse(time.RFC3339Nano, "2026-04-30T08:00:00.000Z")
	if !events[0].Timestamp.Equal(wantTS) {
		t.Errorf("events[0].Timestamp = %v; want %v", events[0].Timestamp, wantTS)
	}
}

// TestEvents_DeduplicatesByMessageID asserts that assistant events sharing the
// same message.id (branch duplicates in Claude Code's conversation DAG) are
// counted only once, preventing inflated usage numbers.
//
// Regression test for Bug 2: Claude Code writes the same API response to every
// branch of the conversation tree.  Without deduplication, a session with 161
// unique API calls but 558 raw lines would inflate token counts by ~3.5×.
func TestEvents_DeduplicatesByMessageID(t *testing.T) {
	t.Parallel()

	const sessionID = "dedup-session"
	dir := t.TempDir()

	encoded := "-test-dedup"
	projectDir := filepath.Join(dir, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Three assistant events: msg_A appears twice (branch duplicate), msg_B once.
	// Expected: 2 unique events (msg_A counted once, msg_B once).
	// Token sum: msg_A (in=10, out=5) + msg_B (in=20, out=8) = in=30, out=13.
	msgA1 := `{"type":"assistant","uuid":"a1","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"dedup-session","parentUuid":"","message":{"model":"claude-opus-4-7","id":"msg_A","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	msgA2 := `{"type":"assistant","uuid":"a2","timestamp":"2026-04-30T10:00:01.000Z","sessionId":"dedup-session","parentUuid":"u2","message":{"model":"claude-opus-4-7","id":"msg_A","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	msgB := `{"type":"assistant","uuid":"b1","timestamp":"2026-04-30T10:01:00.000Z","sessionId":"dedup-session","parentUuid":"a1","message":{"model":"claude-opus-4-7","id":"msg_B","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":20,"output_tokens":8,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	content := msgA1 + "\n" + msgA2 + "\n" + msgB + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}

	// All three events are kept so downstream consumers (timeline, tool
	// extraction) can see every content block — including content splits
	// where Claude Code emits a single API call as multiple JSONL lines
	// sharing the same message.id. The dedup invariant is enforced on
	// USAGE, not event count: subsequent occurrences of the same message
	// ID have their per-event Usage zeroed.
	if len(events) != 3 {
		t.Fatalf("Events() returned %d events; want 3 (all kept, only usage deduped)", len(events))
	}

	// Verify usage totals: msg_A counted once (in=10, out=5) + msg_B once
	// (in=20, out=8) = in=30, out=13. The duplicate msg_A2 must contribute
	// zero, otherwise inflated totals make the usage panel lie.
	var totalIn, totalOut int64
	for _, ev := range events {
		if ap, ok := ev.Payload.(event.AssistantPayload); ok {
			totalIn += ap.Usage.Input
			totalOut += ap.Usage.Output
		}
	}
	if totalIn != 30 {
		t.Errorf("total input tokens = %d; want 30 (dedup prevents counting msg_A twice)", totalIn)
	}
	if totalOut != 13 {
		t.Errorf("total output tokens = %d; want 13", totalOut)
	}
}

// TestEvents_PreservesContentSplitToolUse is the regression test for the
// "activity panel empty after switching session" bug. Claude Code's
// streaming output emits a single assistant API response as MULTIPLE
// JSONL lines — one per content block (thinking, text, tool_use, ...) —
// with the same message.id but a chained parent_uuid (each line's
// parent points at the previous line's uuid). Previously we deduped
// strictly by message.id and dropped the later lines, losing tool_use
// blocks (and their tool calls) for any turn that started with thinking.
//
// New behavior: keep ALL events so the timeline / tool-call extractor
// sees every content block; only zero per-event Usage on duplicates so
// totals don't inflate.
func TestEvents_PreservesContentSplitToolUse(t *testing.T) {
	t.Parallel()

	const sessionID = "split-session"
	dir := t.TempDir()

	encoded := "-test-split"
	projectDir := filepath.Join(dir, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Two assistant events with same message.id, sequential parent chain
	// (line2 parent = line1 uuid). Real Claude Code 4.x emission shape.
	thinkLine := `{"type":"assistant","uuid":"thinkuuid","timestamp":"2026-05-02T12:00:00.000Z","sessionId":"split-session","parentUuid":"u-prev","message":{"model":"claude-opus-4-7","id":"msg_SPLIT","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"considering..."}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	toolLine := `{"type":"assistant","uuid":"tooluuid","timestamp":"2026-05-02T12:00:00.500Z","sessionId":"split-session","parentUuid":"thinkuuid","message":{"model":"claude-opus-4-7","id":"msg_SPLIT","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_01abc","name":"ToolSearch","input":{"q":"x"}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	content := thinkLine + "\n" + toolLine + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}

	// Both lines must be present.
	if len(events) != 2 {
		t.Fatalf("Events() returned %d events; want 2 (both content blocks kept)", len(events))
	}

	// The tool_use line MUST carry its ToolUseID through to the payload —
	// without it the timeline shows zero calls.
	var found bool
	for _, ev := range events {
		ap, ok := ev.Payload.(event.AssistantPayload)
		if !ok {
			continue
		}
		if ap.ToolUseID == "toolu_01abc" && ap.ToolName == "ToolSearch" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a kept event with ToolUseID=toolu_01abc, ToolName=ToolSearch; got %+v", events)
	}

	// Usage must NOT double-count: msg_SPLIT contributes once (input=100, output=50).
	// If both events kept their full usage, we'd see input=200 and the cache /
	// session-context bars would lie.
	var totalIn, totalOut int64
	for _, ev := range events {
		if ap, ok := ev.Payload.(event.AssistantPayload); ok {
			totalIn += ap.Usage.Input
			totalOut += ap.Usage.Output
		}
	}
	if totalIn != 100 {
		t.Errorf("totalIn = %d; want 100 (split must not double-count usage)", totalIn)
	}
	if totalOut != 50 {
		t.Errorf("totalOut = %d; want 50", totalOut)
	}
}

// TestEvents_1hCacheDetects1MContext asserts that an assistant event with
// ephemeral_1h_input_tokens > 0 sets Has1hCache=true on the AssistantPayload.
//
// Regression test for Bug 1: the 1M context window variant of Claude models
// (available to Max plan subscribers) does NOT appear in the JSONL message.model
// field with a "[1m]" suffix.  Instead, it is detected via the 1-hour prompt
// cache tier, which is exclusively available to 1M-context sessions.
func TestEvents_1hCacheDetects1MContext(t *testing.T) {
	t.Parallel()

	const sessionID = "1h-cache-session"
	dir := t.TempDir()

	encoded := "-test-1h-cache"
	projectDir := filepath.Join(dir, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Event with 1h cache tokens (Max plan / 1M context indicator).
	with1h := `{"type":"assistant","uuid":"uuid-1h","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"1h-cache-session","parentUuid":"","message":{"model":"claude-opus-4-7","id":"msg_1h","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":6,"output_tokens":100,"cache_creation_input_tokens":24944,"cache_read_input_tokens":15370,"cache_creation":{"ephemeral_1h_input_tokens":24944,"ephemeral_5m_input_tokens":0}}}}`
	// Event with ONLY 5m cache tokens (standard plan / 200k context).
	with5m := `{"type":"assistant","uuid":"uuid-5m","timestamp":"2026-04-30T10:01:00.000Z","sessionId":"1h-cache-session","parentUuid":"uuid-1h","message":{"model":"claude-opus-4-7","id":"msg_5m","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":6,"output_tokens":50,"cache_creation_input_tokens":1000,"cache_read_input_tokens":500,"cache_creation":{"ephemeral_1h_input_tokens":0,"ephemeral_5m_input_tokens":1000}}}}`
	content := with1h + "\n" + with5m + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("Events() returned %d events; want 2", len(events))
	}

	// First event: has 1h cache → Has1hCache must be true.
	ap0, ok := events[0].Payload.(event.AssistantPayload)
	if !ok {
		t.Fatalf("events[0].Payload is %T; want AssistantPayload", events[0].Payload)
	}
	if !ap0.Has1hCache {
		t.Error("events[0]: Has1hCache should be true (ephemeral_1h_input_tokens=24944)")
	}

	// Second event: only 5m cache → Has1hCache must be false.
	ap1, ok := events[1].Payload.(event.AssistantPayload)
	if !ok {
		t.Fatalf("events[1].Payload is %T; want AssistantPayload", events[1].Payload)
	}
	if ap1.Has1hCache {
		t.Error("events[1]: Has1hCache should be false (ephemeral_1h_input_tokens=0)")
	}
}

// TestEvents_ModelFieldExtracted asserts that the "message.model" field from an
// assistant JSONL event is populated into AssistantPayload.Model (Phase C).
func TestEvents_ModelFieldExtracted(t *testing.T) {
	t.Parallel()

	const sessionID = "model-field-session"
	dir := t.TempDir()

	encoded := "-test-model"
	projectDir := filepath.Join(dir, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Assistant event with model field set to "claude-opus-4-7".
	line := `{"type":"assistant","uuid":"asst-model-01","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"model-field-session","parentUuid":"","message":{"model":"claude-opus-4-7","id":"msg_01","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	content := line + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("Events() returned %d events; want 1", len(events))
	}

	ap, ok := events[0].Payload.(event.AssistantPayload)
	if !ok {
		t.Fatalf("Payload is %T; want AssistantPayload", events[0].Payload)
	}
	if ap.Model != "claude-opus-4-7" {
		t.Errorf("AssistantPayload.Model = %q; want %q", ap.Model, "claude-opus-4-7")
	}
}
