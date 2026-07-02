package git

import "testing"

// FuzzParseStatus exercises the porcelain --status=v1 parser against
// arbitrary input. Contract: must not panic.
func FuzzParseStatus(f *testing.F) {
	// -z (NUL-separated) form, which is what Status now requests.
	f.Add([]byte(" M file.go\x00"))
	f.Add([]byte("?? new.go\x00"))
	f.Add([]byte("MM modified.go\x00A  added.go\x00D  deleted.go\x00"))
	f.Add([]byte("R  new.go\x00old.go\x00")) // rename: trailing old-path field
	f.Add([]byte("?? café.txt\x00"))         // non-ASCII, verbatim
	// Legacy newline form and junk — must still not panic.
	f.Add([]byte(" M file.go\n"))
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
