package processscan

import "testing"

// FuzzParseClaudeSessionIDs exercises the ps-output parser against
// arbitrary bytes. Contract: must not panic on any input, including
// truncated, embedded-NUL, or non-UTF-8 bytes that ps could emit on
// strange systems.
func FuzzParseClaudeSessionIDs(f *testing.F) {
	f.Add([]byte("claude --session-id 12345678-1234-1234-1234-123456789012\n"))
	f.Add([]byte(""))
	f.Add([]byte("noise without session"))
	f.Add([]byte("claude --session-id\n"))
	f.Add([]byte("claude --session-id "))
	f.Add([]byte("\x00\x00\x00"))

	f.Fuzz(func(_ *testing.T, raw []byte) {
		_ = parseClaudeSessionIDs(raw)
	})
}
