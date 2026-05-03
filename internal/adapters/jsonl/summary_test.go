package jsonl_test

// summary_test.go — TDD tests for Phase 3 Summary extraction.
// Tests in this file correspond to tasks 3.1, 3.3, and 3.5 from tasks.md.
// All tests use decodeLine via the public Source.Events() surface or by
// constructing inline JSONL lines and routing through the public API.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/clyde-tui/clyde/internal/adapters/jsonl"
	"github.com/clyde-tui/clyde/internal/domain/event"
	"github.com/clyde-tui/clyde/internal/domain/session"
)

// helper to set up a Source that serves a single-line JSONL fixture.
// Returns the events decoded from that line.
func decodeLines(t *testing.T, lines ...string) []event.Event {
	t.Helper()
	const sessionID = "test-session"

	dir := t.TempDir()
	projectDir := filepath.Join(dir, "-test")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	events, err := src.Events(context.Background(), session.ID(sessionID))
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}
	return events
}

// ─── Phase 3.1 — Failing tests for extractUserSummary ───────────────────────

// TestDecodeLine_UserSummaryFromString: user event with message.content as a JSON string
// → UserPayload.Summary equals truncated string; IsToolResultOnly = false.
func TestDecodeLine_UserSummaryFromString(t *testing.T) {
	t.Parallel()

	line := `{"type":"user","uuid":"u1","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","message":{"role":"user","content":"Hello, Claude!"}}`
	events := decodeLines(t, line)

	if len(events) != 1 {
		t.Fatalf("got %d events; want 1", len(events))
	}

	up, ok := events[0].Payload.(event.UserPayload)
	if !ok {
		t.Fatalf("Payload is %T; want event.UserPayload", events[0].Payload)
	}

	if up.Summary != "Hello, Claude!" {
		t.Errorf("Summary = %q; want %q", up.Summary, "Hello, Claude!")
	}
	if up.IsToolResultOnly {
		t.Errorf("IsToolResultOnly = true; want false for string content")
	}
}

// TestDecodeLine_UserSummaryFromTextBlock: content is array with single text block.
func TestDecodeLine_UserSummaryFromTextBlock(t *testing.T) {
	t.Parallel()

	line := `{"type":"user","uuid":"u2","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}`
	events := decodeLines(t, line)

	if len(events) != 1 {
		t.Fatalf("got %d events; want 1", len(events))
	}

	up, ok := events[0].Payload.(event.UserPayload)
	if !ok {
		t.Fatalf("Payload is %T; want event.UserPayload", events[0].Payload)
	}

	if up.Summary != "Hello" {
		t.Errorf("Summary = %q; want %q", up.Summary, "Hello")
	}
	if up.IsToolResultOnly {
		t.Errorf("IsToolResultOnly = true; want false for text block content")
	}
}

// TestDecodeLine_UserIsMetaFlag: JSONL line with top-level "isMeta":true
// → UserPayload.IsMeta = true.
func TestDecodeLine_UserIsMetaFlag(t *testing.T) {
	t.Parallel()

	line := `{"type":"user","uuid":"u3","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","isMeta":true,"message":{"role":"user","content":"system prompt"}}`
	events := decodeLines(t, line)

	if len(events) != 1 {
		t.Fatalf("got %d events; want 1", len(events))
	}

	up, ok := events[0].Payload.(event.UserPayload)
	if !ok {
		t.Fatalf("Payload is %T; want event.UserPayload", events[0].Payload)
	}

	if !up.IsMeta {
		t.Errorf("IsMeta = false; want true for isMeta:true envelope")
	}
}

// TestDecodeLine_UserIsToolResultOnly: content array of exclusively tool_result blocks
// → IsToolResultOnly = true, Summary = "".
func TestDecodeLine_UserIsToolResultOnly(t *testing.T) {
	t.Parallel()

	line := `{"type":"user","uuid":"u4","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"result1"},{"type":"tool_result","tool_use_id":"toolu_02","content":"result2"}]}}`
	events := decodeLines(t, line)

	if len(events) != 1 {
		t.Fatalf("got %d events; want 1", len(events))
	}

	up, ok := events[0].Payload.(event.UserPayload)
	if !ok {
		t.Fatalf("Payload is %T; want event.UserPayload", events[0].Payload)
	}

	if !up.IsToolResultOnly {
		t.Errorf("IsToolResultOnly = false; want true for tool_result-only content")
	}
	if up.Summary != "" {
		t.Errorf("Summary = %q; want empty for tool_result-only content", up.Summary)
	}
}

// TestDecodeLine_UserMixedNotToolResultOnly: content array with text + tool_result
// → IsToolResultOnly = false, Summary non-empty.
func TestDecodeLine_UserMixedNotToolResultOnly(t *testing.T) {
	t.Parallel()

	line := `{"type":"user","uuid":"u5","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","message":{"role":"user","content":[{"type":"text","text":"Here are the results:"},{"type":"tool_result","tool_use_id":"toolu_01","content":"result data"}]}}`
	events := decodeLines(t, line)

	if len(events) != 1 {
		t.Fatalf("got %d events; want 1", len(events))
	}

	up, ok := events[0].Payload.(event.UserPayload)
	if !ok {
		t.Fatalf("Payload is %T; want event.UserPayload", events[0].Payload)
	}

	if up.IsToolResultOnly {
		t.Errorf("IsToolResultOnly = true; want false for mixed text+tool_result content")
	}
	if up.Summary == "" {
		t.Errorf("Summary is empty; want non-empty for mixed content with text block")
	}
	if up.Summary != "Here are the results:" {
		t.Errorf("Summary = %q; want %q", up.Summary, "Here are the results:")
	}
}

// TestExtractUserSummary_EmptyContentArray: content is empty array []
// → IsToolResultOnly = false, Summary = "".
func TestExtractUserSummary_EmptyContentArray(t *testing.T) {
	t.Parallel()

	line := `{"type":"user","uuid":"u6","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","message":{"role":"user","content":[]}}`
	events := decodeLines(t, line)

	if len(events) != 1 {
		t.Fatalf("got %d events; want 1", len(events))
	}

	up, ok := events[0].Payload.(event.UserPayload)
	if !ok {
		t.Fatalf("Payload is %T; want event.UserPayload", events[0].Payload)
	}

	if up.IsToolResultOnly {
		t.Errorf("IsToolResultOnly = true; want false for empty content array")
	}
	if up.Summary != "" {
		t.Errorf("Summary = %q; want empty for empty content array", up.Summary)
	}
}

// ─── Phase 3.3 — Failing tests for extractAssistantSummary ──────────────────

// TestDecodeLine_AssistantTextSummary: single text block → AssistantPayload.Summary equals text.
func TestDecodeLine_AssistantTextSummary(t *testing.T) {
	t.Parallel()

	line := `{"type":"assistant","uuid":"a1","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","message":{"model":"claude-opus-4-7","id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"Let me check the file for you."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":8,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	events := decodeLines(t, line)

	if len(events) != 1 {
		t.Fatalf("got %d events; want 1", len(events))
	}

	ap, ok := events[0].Payload.(event.AssistantPayload)
	if !ok {
		t.Fatalf("Payload is %T; want event.AssistantPayload", events[0].Payload)
	}

	const want = "Let me check the file for you."
	if ap.Summary != want {
		t.Errorf("Summary = %q; want %q", ap.Summary, want)
	}
}

// TestDecodeLine_AssistantToolUseSummary: table-driven test for tool_use priority.
func TestDecodeLine_AssistantToolUseSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentJSON string
		wantSummary string
	}{
		{
			name:        "single_tool_use_read",
			contentJSON: `[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"/some/file.go"}}]`,
			wantSummary: "Tool: Read /some/file.go",
		},
		{
			name:        "multiple_tool_use_first_wins",
			contentJSON: `[{"type":"tool_use","id":"toolu_1","name":"Edit","input":{"file_path":"/a.go"}},{"type":"tool_use","id":"toolu_2","name":"Read","input":{"file_path":"/b.go"}}]`,
			wantSummary: "Tool: Edit /a.go",
		},
		{
			name:        "text_and_tool_use_tool_wins",
			contentJSON: `[{"type":"text","text":"I will read the file."},{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"/x.go"}}]`,
			wantSummary: "Tool: Read /x.go",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			line := fmt.Sprintf(
				`{"type":"assistant","uuid":"a-%s","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","message":{"model":"claude","id":"msg_1","type":"message","role":"assistant","content":%s,"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
				tc.name, tc.contentJSON,
			)
			events := decodeLines(t, line)

			if len(events) != 1 {
				t.Fatalf("got %d events; want 1", len(events))
			}

			ap, ok := events[0].Payload.(event.AssistantPayload)
			if !ok {
				t.Fatalf("Payload is %T; want event.AssistantPayload", events[0].Payload)
			}

			if ap.Summary != tc.wantSummary {
				t.Errorf("Summary = %q; want %q", ap.Summary, tc.wantSummary)
			}
		})
	}
}

// TestDecodeLine_AssistantThinkingOnly: only thinking block → Summary = "(thinking)".
func TestDecodeLine_AssistantThinkingOnly(t *testing.T) {
	t.Parallel()

	line := `{"type":"assistant","uuid":"a2","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","message":{"model":"claude-opus-4-7","id":"msg_2","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"Let me think..."}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	events := decodeLines(t, line)

	if len(events) != 1 {
		t.Fatalf("got %d events; want 1", len(events))
	}

	ap, ok := events[0].Payload.(event.AssistantPayload)
	if !ok {
		t.Fatalf("Payload is %T; want event.AssistantPayload", events[0].Payload)
	}

	const want = "(thinking)"
	if ap.Summary != want {
		t.Errorf("Summary = %q; want %q", ap.Summary, want)
	}
}

// ─── Phase 3.5 — Failing tests for extractToolSummary (full table) ───────────

// TestExtractToolSummary: table-driven test covering every entry from the locked
// per-tool format table in spec/design.
func TestExtractToolSummary(t *testing.T) {
	t.Parallel()

	// Helper to build an assistant JSONL line with a single tool_use block.
	buildToolUseLine := func(name string, inputJSON string) string {
		return fmt.Sprintf(
			`{"type":"assistant","uuid":"a-%s","timestamp":"2026-04-30T10:00:00.000Z","sessionId":"test-session","parentUuid":"","message":{"model":"claude","id":"msg_1","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":%q,"input":%s}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
			name, name, inputJSON,
		)
	}

	// Build a 100-rune command for bash truncation test.
	longCommand := ""
	for i := 0; i < 100; i++ {
		longCommand += "x"
	}
	// After inner truncation at 60 runes: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" (60 x's)
	// wrapped: "Tool: Bash 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'"
	// That's "Tool: Bash '" (12) + 60 + "'" (1) = 73 chars — under 80, no outer truncation.
	inner60 := longCommand[:60]

	tests := []struct {
		name        string
		toolName    string
		inputJSON   string
		wantSummary string
	}{
		{
			name:        "read_file_path",
			toolName:    "Read",
			inputJSON:   `{"file_path":"/a/b.go"}`,
			wantSummary: "Tool: Read /a/b.go",
		},
		{
			name:        "write_file_path",
			toolName:    "Write",
			inputJSON:   `{"file_path":"/a/b.go","content":"hello"}`,
			wantSummary: "Tool: Write /a/b.go",
		},
		{
			name:        "edit_file_path",
			toolName:    "Edit",
			inputJSON:   `{"file_path":"/a/b.go","old_string":"x","new_string":"y"}`,
			wantSummary: "Tool: Edit /a/b.go",
		},
		{
			name:        "multiedit_file_path",
			toolName:    "MultiEdit",
			inputJSON:   `{"file_path":"/a/b.go","edits":[]}`,
			wantSummary: "Tool: MultiEdit /a/b.go",
		},
		{
			name:        "bash_with_description",
			toolName:    "Bash",
			inputJSON:   `{"description":"Run tests","command":"go test ./..."}`,
			wantSummary: "Tool: Bash Run tests",
		},
		{
			name:        "bash_command_short",
			toolName:    "Bash",
			inputJSON:   `{"command":"go test ./..."}`,
			wantSummary: "Tool: Bash 'go test ./...'",
		},
		{
			name:        "bash_command_long_truncated",
			toolName:    "Bash",
			inputJSON:   fmt.Sprintf(`{"command":%q}`, longCommand),
			wantSummary: fmt.Sprintf("Tool: Bash '%s'", inner60),
		},
		{
			name:        "grep_pattern",
			toolName:    "Grep",
			inputJSON:   `{"pattern":"func main"}`,
			wantSummary: "Tool: Grep func main",
		},
		{
			name:        "rg_maps_to_grep_label",
			toolName:    "rg",
			inputJSON:   `{"pattern":"func main"}`,
			wantSummary: "Tool: Grep func main",
		},
		{
			name:        "glob_pattern",
			toolName:    "Glob",
			inputJSON:   `{"pattern":"**/*.go"}`,
			wantSummary: "Tool: Glob **/*.go",
		},
		{
			name:        "task_description",
			toolName:    "Task",
			inputJSON:   `{"description":"Implement feature X"}`,
			wantSummary: "Tool: Task Implement feature X",
		},
		{
			name:        "agent_maps_to_task_label",
			toolName:    "Agent",
			inputJSON:   `{"description":"Run sub-task"}`,
			wantSummary: "Tool: Task Run sub-task",
		},
		{
			name:        "todowrite_n_items",
			toolName:    "TodoWrite",
			inputJSON:   `{"todos":[{"id":"1","content":"task1"},{"id":"2","content":"task2"},{"id":"3","content":"task3"}]}`,
			wantSummary: "Tool: TodoWrite (3 items)",
		},
		{
			name:        "todowrite_zero_items",
			toolName:    "TodoWrite",
			inputJSON:   `{"todos":[]}`,
			wantSummary: "Tool: TodoWrite (0 items)",
		},
		{
			name:        "unknown_tool_name_only",
			toolName:    "ImaginaryTool",
			inputJSON:   `{}`,
			wantSummary: "Tool: ImaginaryTool",
		},
		{
			name:        "mcp_default_branch",
			toolName:    "mcp__server__action",
			inputJSON:   `{}`,
			wantSummary: "Tool: mcp__server__action",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			line := buildToolUseLine(tc.toolName, tc.inputJSON)
			events := decodeLines(t, line)

			if len(events) != 1 {
				t.Fatalf("got %d events; want 1", len(events))
			}

			ap, ok := events[0].Payload.(event.AssistantPayload)
			if !ok {
				t.Fatalf("Payload is %T; want event.AssistantPayload", events[0].Payload)
			}

			if ap.Summary != tc.wantSummary {
				t.Errorf("Summary = %q; want %q", ap.Summary, tc.wantSummary)
			}
		})
	}
}
