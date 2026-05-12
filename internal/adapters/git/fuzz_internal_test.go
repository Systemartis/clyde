package git

import "testing"

// FuzzParseStatus exercises the porcelain --status=v1 parser against
// arbitrary input. Contract: must not panic.
func FuzzParseStatus(f *testing.F) {
	f.Add([]byte(" M file.go\n"))
	f.Add([]byte("?? new.go\n"))
	f.Add([]byte("MM modified.go\nA  added.go\nD  deleted.go\n"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\xff"))

	f.Fuzz(func(_ *testing.T, raw []byte) {
		_ = parseStatus(raw)
	})
}

// FuzzParseDiff exercises the unified-diff parser against arbitrary
// input. Contract: must not panic.
func FuzzParseDiff(f *testing.F) {
	f.Add([]byte("@@ -1,3 +1,3 @@\n line\n-old\n+new\n"))
	f.Add([]byte("@@ -1 +1 @@\n-a\n+b\n"))
	f.Add([]byte("not a diff at all"))
	f.Add([]byte(""))
	f.Add([]byte("@@\n"))
	f.Add([]byte("@@ -0,0 +1 @@\n+only\n"))

	f.Fuzz(func(_ *testing.T, raw []byte) {
		_ = parseDiff(raw)
	})
}
