package git

import "testing"

// TestParseStatus_NULSeparated is the regression for the LOW finding: without
// -z, `git status --porcelain` C-quotes non-ASCII / space-containing paths
// (e.g. "caf\303\251.txt"), and parseStatus never unquoted them. The fix
// switches Status to `-z` output; parseStatus now splits on NUL and takes the
// path verbatim (no quoting, no TrimSpace mangling).
func TestParseStatus_NULSeparated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  []byte
		want []FileStatus
	}{
		{
			name: "untracked non-ascii path preserved verbatim",
			raw:  []byte("?? café.txt\x00"),
			want: []FileStatus{{Path: "café.txt", Status: '?', Staged: false}},
		},
		{
			name: "untracked path with a space preserved verbatim",
			raw:  []byte("?? with space.txt\x00"),
			want: []FileStatus{{Path: "with space.txt", Status: '?', Staged: false}},
		},
		{
			name: "modified unstaged",
			raw:  []byte(" M mod.go\x00"),
			want: []FileStatus{{Path: "mod.go", Status: 'M', Staged: false}},
		},
		{
			name: "staged add",
			raw:  []byte("A  add.go\x00"),
			want: []FileStatus{{Path: "add.go", Status: 'A', Staged: true}},
		},
		{
			name: "staged rename keeps new path, discards trailing old-path field",
			raw:  []byte("R  renamed.txt\x00orig.txt\x00"),
			want: []FileStatus{{Path: "renamed.txt", Status: 'R', Staged: true}},
		},
		{
			name: "rename with non-ascii new path",
			raw:  []byte("R  café.txt\x00orig.txt\x00"),
			want: []FileStatus{{Path: "café.txt", Status: 'R', Staged: true}},
		},
		{
			name: "mixed stream in one -z buffer",
			raw:  []byte(" M a.go\x00?? b space.go\x00R  d.go\x00c.go\x00A  e.go\x00"),
			want: []FileStatus{
				{Path: "a.go", Status: 'M', Staged: false},
				{Path: "b space.go", Status: '?', Staged: false},
				{Path: "d.go", Status: 'R', Staged: true},
				{Path: "e.go", Status: 'A', Staged: true},
			},
		},
		{
			name: "empty input",
			raw:  []byte(""),
			want: nil,
		},
		{
			name: "trailing NUL only yields no entries",
			raw:  []byte("\x00"),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseStatus(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("parseStatus() len = %d, want %d (got %+v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("entry[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
