package jsonl

import "strings"

// encodeProjectPath converts an absolute project CWD to the directory name
// used by Claude Code under ~/.claude/projects/.
//
// Algorithm (observational — not Anthropic-documented):
//
//	Replace every character that is not [A-Za-z0-9-] with '-'.
//	Existing dashes in directory names are preserved.
//
// Examples (real evidence from ~/.claude/projects/):
//
//	"/Users/vladpb/work/Personal"
//	  → "-Users-vladpb-work-Personal"
//
//	"/Users/vladpb/work/Personal/diary2/.claude/worktrees/bold-villani"
//	  → "-Users-vladpb-work-Personal-diary2--claude-worktrees-bold-villani"
//
//	"/Users/vladpb/Library/Application Support/CodexBar/ClaudeProbe"
//	  → "-Users-vladpb-Library-Application-Support-CodexBar-ClaudeProbe"
//
// The encoding lives in exactly one function so that if Anthropic ever changes
// the scheme, exactly one place changes and the fixture tests break loudly.
func encodeProjectPath(cwd string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-':
			return r
		default:
			return '-'
		}
	}, cwd)
}
