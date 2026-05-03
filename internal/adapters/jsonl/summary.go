package jsonl

// summary.go — JSONL Summary extraction helpers for Phase 3 (ADR-010).
//
// Three private functions implement the ADR-009 / ADR-010 extraction contracts:
//
//   - extractUserSummary  — dispatches on string vs array content for user events
//   - extractAssistantSummary — implements the tool_use > text > thinking priority chain
//   - extractToolSummary — formats a single tool_use block per the locked per-tool table
//
// All helpers are pure functions: they take json.RawMessage in, return strings/bools out.
// No I/O. Depguard domain-pure compliance: only encoding/json, fmt, strings, and the
// domain event package (for event.Truncate) are imported.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/clyde-tui/clyde/internal/domain/event"
)

// ─── Content block types ─────────────────────────────────────────────────────

// contentBlock is a minimal decode of a single element in a message.content array.
// Only the "type" field is always required; other fields are tool-specific.
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`        // present on "text" blocks
	Name      string          `json:"name"`        // present on "tool_use" blocks
	ID        string          `json:"id"`          // present on "tool_use" blocks (the tool_use_id)
	Input     json.RawMessage `json:"input"`       // present on "tool_use" blocks
	ToolUseID string          `json:"tool_use_id"` // present on "tool_result" blocks
	IsError   bool            `json:"is_error"`    // present on "tool_result" blocks
}

// ─── extractUserSummary ──────────────────────────────────────────────────────

// extractUserSummary returns the displayable text and IsToolResultOnly flag for
// a user event's message.content. content may be a JSON string or an array of
// blocks (ADR-009 algorithm).
//
// Returns:
//
//	summary         — truncated display text (may be empty)
//	toolResultOnly  — true iff content is an array of exclusively tool_result blocks
func extractUserSummary(content json.RawMessage) (summary string, toolResultOnly bool) {
	if len(content) == 0 {
		return "", false
	}

	// Detect whether content is a JSON string or an array by the first byte.
	switch content[0] {
	case '"':
		// String content — typed user prompt.
		var s string
		if err := json.Unmarshal(content, &s); err != nil {
			return "", false
		}
		return event.Truncate(s, 80), false

	case '[':
		return extractUserSummaryFromArray(content)

	default:
		// Unexpected shape — return empty, not an error (best-effort).
		return "", false
	}
}

// extractUserSummaryFromArray handles the array-of-blocks variant of user content.
func extractUserSummaryFromArray(content json.RawMessage) (summary string, toolResultOnly bool) {
	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return "", false
	}

	if len(blocks) == 0 {
		// Empty array — degenerate case; not tool_result-only (no blocks at all).
		return "", false
	}

	// Check if ALL blocks are tool_result.
	allToolResult := true
	for _, b := range blocks {
		if b.Type != "tool_result" {
			allToolResult = false
			break
		}
	}
	if allToolResult {
		return "", true
	}

	// Mixed or text-only — find first text block.
	for _, b := range blocks {
		if b.Type == "text" {
			return event.Truncate(b.Text, 80), false
		}
	}

	// No text block found but not all tool_result (e.g. all "image" blocks).
	return "", false
}

// extractToolResultInfo extracts tool_use_id values and error flag from user content.
// Returns (toolUseIDs, hasError). Called only when content is known to be an array.
func extractToolResultInfo(content json.RawMessage) (toolUseIDs []string, hasError bool) {
	if len(content) == 0 || content[0] != '[' {
		return nil, false
	}
	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, false
	}
	for _, b := range blocks {
		if b.Type == "tool_result" && b.ToolUseID != "" {
			toolUseIDs = append(toolUseIDs, b.ToolUseID)
			if b.IsError {
				hasError = true
			}
		}
	}
	return toolUseIDs, hasError
}

// extractAssistantToolUse extracts the ID and name of the first tool_use block.
// Returns ("", "") when no tool_use block is present.
func extractAssistantToolUse(content json.RawMessage) (toolUseID, toolName string) {
	if len(content) == 0 {
		return "", ""
	}
	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return "", ""
	}
	for _, b := range blocks {
		if b.Type == "tool_use" && b.ID != "" {
			return b.ID, b.Name
		}
	}
	return "", ""
}

// ─── extractAssistantSummary ─────────────────────────────────────────────────

// extractAssistantSummary implements the priority chain for assistant events
// (ADR-010): tool_use > text > thinking > "".
//
// Split into helpers to stay below gocyclo 15 per design risk note.
func extractAssistantSummary(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}

	// Priority 1: first tool_use block.
	if b := firstToolUseBlock(blocks); b != nil {
		return event.Truncate(extractToolSummary(b.Name, b.Input), 80)
	}

	// Priority 2: first text block.
	if text := firstTextBlock(blocks); text != "" {
		return event.Truncate(text, 80)
	}

	// Priority 3: any thinking block.
	if hasThinkingBlock(blocks) {
		return "(thinking)"
	}

	return ""
}

// firstToolUseBlock returns the first block with type "tool_use", or nil.
func firstToolUseBlock(blocks []contentBlock) *contentBlock {
	for i := range blocks {
		if blocks[i].Type == "tool_use" {
			return &blocks[i]
		}
	}
	return nil
}

// firstTextBlock returns the text field of the first block with type "text", or "".
func firstTextBlock(blocks []contentBlock) string {
	for _, b := range blocks {
		if b.Type == "text" {
			return b.Text
		}
	}
	return ""
}

// hasThinkingBlock returns true if any block has type "thinking".
func hasThinkingBlock(blocks []contentBlock) bool {
	for _, b := range blocks {
		if b.Type == "thinking" {
			return true
		}
	}
	return false
}

// ─── extractToolSummary ──────────────────────────────────────────────────────

// extractToolSummary formats a single tool_use block per the locked per-tool
// summary table (design.md ADR-010). The outer Truncate(80) is applied by the
// caller (extractAssistantSummary).
//
// The function itself does NOT apply the outer Truncate — it returns the raw
// format string. This allows the caller to apply Truncate once consistently.
func extractToolSummary(name string, input json.RawMessage) string {
	switch name {
	case "Read", "Write", "Edit", "MultiEdit":
		return extractFilePathTool(name, input)

	case "Bash":
		return extractBashTool(input)

	case "Grep", "rg":
		return extractPatternTool("Grep", input)

	case "Glob":
		return extractPatternTool("Glob", input)

	case "Task", "Agent":
		return extractDescriptionTool("Task", input)

	case "TodoWrite":
		return extractTodoWriteTool(input)

	default:
		// MCP tools (mcp__*) and any unknown tool.
		return "Tool: " + name
	}
}

// extractFilePathTool handles Read, Write, Edit, MultiEdit: "Tool: <Name> <file_path>".
func extractFilePathTool(label string, input json.RawMessage) string {
	var args struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &args); err != nil || args.FilePath == "" {
		return "Tool: " + label
	}
	return "Tool: " + label + " " + args.FilePath
}

// extractBashTool handles Bash: description takes priority over command.
// Command is inner-truncated at 60 runes and wrapped in single quotes.
func extractBashTool(input json.RawMessage) string {
	var args struct {
		Description string `json:"description"`
		Command     string `json:"command"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "Tool: Bash"
	}

	if strings.TrimSpace(args.Description) != "" {
		return "Tool: Bash " + args.Description
	}

	// No description — use command, inner-truncated at 60 runes.
	cmd := truncateRunes(args.Command, 60)
	return fmt.Sprintf("Tool: Bash '%s'", cmd)
}

// extractPatternTool handles Grep/rg and Glob: "Tool: <label> <pattern>".
func extractPatternTool(label string, input json.RawMessage) string {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &args); err != nil || args.Pattern == "" {
		return "Tool: " + label
	}
	return "Tool: " + label + " " + args.Pattern
}

// extractDescriptionTool handles Task/Agent: "Tool: Task <description>".
func extractDescriptionTool(label string, input json.RawMessage) string {
	var args struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &args); err != nil || args.Description == "" {
		return "Tool: " + label
	}
	return "Tool: " + label + " " + args.Description
}

// extractTodoWriteTool handles TodoWrite: "Tool: TodoWrite (<n> items)".
func extractTodoWriteTool(input json.RawMessage) string {
	var args struct {
		Todos []json.RawMessage `json:"todos"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "Tool: TodoWrite (0 items)"
	}
	return fmt.Sprintf("Tool: TodoWrite (%d items)", len(args.Todos))
}

// ─── truncateRunes ────────────────────────────────────────────────────────────

// truncateRunes returns the first maxRunes runes of s without whitespace
// normalization or ellipsis. Used for the Bash inner command truncation
// (60-rune cap before quoting). This is intentionally simpler than
// event.Truncate — no normalization, no ellipsis, just a hard rune cut.
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i]
		}
		count++
	}
	return s
}
