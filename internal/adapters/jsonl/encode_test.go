package jsonl

import "testing"

// TestEncodeProjectPath asserts the observational encoding scheme.
//
// Real-world evidence from ~/.claude/projects/ shows Claude Code replaces ANY
// non-alphanumeric character (except '-') with '-'. The cases below pin
// observed real encodings. Any deviation from this set must be challenged.
//
// The encoding is observational (not Anthropic-documented). The single private
// function that implements it is exercised here so any accidental change to
// the algorithm breaks loudly.
func TestEncodeProjectPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		cwd  string
		want string
	}{
		{
			cwd:  "/Users/vladpb/work/Personal",
			want: "-Users-vladpb-work-Personal",
		},
		{
			cwd:  "/Users/vladpb/work/Personal/clyde",
			want: "-Users-vladpb-work-Personal-clyde",
		},
		{
			cwd:  "/home/user/projects/myapp",
			want: "-home-user-projects-myapp",
		},
		{
			cwd:  "/",
			want: "-",
		},
		// Real evidence: dot in path (.claude worktree directory).
		// Observed at ~/.claude/projects/-Users-vladpb-work-Personal-diary2--claude-worktrees-bold-villani
		{
			cwd:  "/Users/vladpb/work/Personal/diary2/.claude/worktrees/bold-villani",
			want: "-Users-vladpb-work-Personal-diary2--claude-worktrees-bold-villani",
		},
		// Real evidence: space in path (macOS Application Support).
		// Observed at ~/.claude/projects/-Users-vladpb-Library-Application-Support-CodexBar-ClaudeProbe
		{
			cwd:  "/Users/vladpb/Library/Application Support/CodexBar/ClaudeProbe",
			want: "-Users-vladpb-Library-Application-Support-CodexBar-ClaudeProbe",
		},
		// Existing dashes in directory names are preserved (not collapsed).
		{
			cwd:  "/Users/vladpb/work/Systemartis/VoiceVA-France",
			want: "-Users-vladpb-work-Systemartis-VoiceVA-France",
		},
	}

	for _, tc := range cases {
		t.Run(tc.cwd, func(t *testing.T) {
			t.Parallel()
			got := encodeProjectPath(tc.cwd)
			if got != tc.want {
				t.Errorf("encodeProjectPath(%q) = %q; want %q", tc.cwd, got, tc.want)
			}
		})
	}
}
