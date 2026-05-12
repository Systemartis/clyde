package jsonl

import "testing"

// BenchmarkEncodeProjectPath measures the cwd → project-dir-name encoding,
// which runs once per session-list refresh. The string-allocation overhead
// matters for users with many sessions; this baseline lets us catch a
// regression if encode.go gains an extra walk.
func BenchmarkEncodeProjectPath(b *testing.B) {
	// Real-world cwd — long enough to exercise the strings.Map allocation
	// without being a degenerate edge case.
	cwd := "/Users/alice/work/Personal/diary2/.claude/worktrees/some-branch"
	for b.Loop() {
		_ = encodeProjectPath(cwd)
	}
}

// BenchmarkEncodeProjectPath_Short measures the small-cwd case for comparison.
func BenchmarkEncodeProjectPath_Short(b *testing.B) {
	cwd := "/tmp"
	for b.Loop() {
		_ = encodeProjectPath(cwd)
	}
}
