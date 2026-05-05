package jsonl_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/adapters/jsonl"
	"github.com/Systemartis/clyde/internal/domain/session"
	"github.com/Systemartis/clyde/internal/ports"
)

// compile-time assertion: *Source satisfies ports.SessionSource.
var _ ports.SessionSource = (*jsonl.Source)(nil)

// compile-time assertion: *Source satisfies ports.SubagentSource.
var _ ports.SubagentSource = (*jsonl.Source)(nil)

// compile-time assertion: *Source satisfies ports.GlobalSessionSource.
var _ ports.GlobalSessionSource = (*jsonl.Source)(nil)

// TestSessions_MultiSession reads two JSONL files from a temp-dir project
// directory and asserts that Sessions() returns both sessions ordered by
// LastActivity descending (most recently active first).
func TestSessions_MultiSession(t *testing.T) {
	t.Parallel()

	// We use a fake project cwd "/test/multi" → encoded "-test-multi".
	const projectCWD = "/test/multi"
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "-test-multi")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// session-a.jsonl has an earlier last event timestamp.
	sessionAID := "session-a"
	sessionAContent := `{"type":"user","uuid":"ua1","timestamp":"2026-04-29T10:00:00.000Z","sessionId":"session-a","parentUuid":""}` + "\n" +
		`{"type":"assistant","uuid":"ua2","timestamp":"2026-04-29T10:01:00.000Z","sessionId":"session-a","parentUuid":"ua1","message":{"model":"claude","id":"msg_a","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"

	// session-b.jsonl has a later last event timestamp.
	sessionBID := "session-b"
	sessionBContent := `{"type":"user","uuid":"ub1","timestamp":"2026-04-30T09:00:00.000Z","sessionId":"session-b","parentUuid":""}` + "\n" +
		`{"type":"assistant","uuid":"ub2","timestamp":"2026-04-30T09:05:00.000Z","sessionId":"session-b","parentUuid":"ub1","message":{"model":"claude","id":"msg_b","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"

	// Write files with deliberate mtime ordering: session-b is newer.
	oldTime := time.Date(2026, 4, 29, 10, 2, 0, 0, time.UTC)
	newTime := time.Date(2026, 4, 30, 9, 6, 0, 0, time.UTC)

	aPath := filepath.Join(projectDir, sessionAID+".jsonl")
	bPath := filepath.Join(projectDir, sessionBID+".jsonl")

	if err := os.WriteFile(aPath, []byte(sessionAContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(aPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(bPath, []byte(sessionBContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(bPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	summaries, err := src.Sessions(context.Background(), projectCWD)
	if err != nil {
		t.Fatalf("Sessions() error: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("Sessions() returned %d summaries; want 2", len(summaries))
	}

	// Most recently active session must be first.
	if summaries[0].ID != session.ID(sessionBID) {
		t.Errorf("summaries[0].ID = %q; want %q", summaries[0].ID, sessionBID)
	}
	if summaries[1].ID != session.ID(sessionAID) {
		t.Errorf("summaries[1].ID = %q; want %q", summaries[1].ID, sessionAID)
	}

	// LastActivity should be the file mtime (at minimum it should not be zero).
	if summaries[0].LastActivity.IsZero() {
		t.Error("summaries[0].LastActivity is zero")
	}
	if summaries[1].LastActivity.IsZero() {
		t.Error("summaries[1].LastActivity is zero")
	}

	// B must have a later LastActivity than A.
	if !summaries[0].LastActivity.After(summaries[1].LastActivity) {
		t.Errorf("summaries[0].LastActivity (%v) should be after summaries[1].LastActivity (%v)",
			summaries[0].LastActivity, summaries[1].LastActivity)
	}
}

// TestSessions_MissingDirectory asserts that Sessions() returns an empty slice
// (NOT an error) when the encoded project directory does not exist.
// This is the expected state when a project has never had a Claude Code session.
func TestSessions_MissingDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := jsonl.NewSource(dir)

	summaries, err := src.Sessions(context.Background(), "/never/had/sessions")
	if err != nil {
		t.Fatalf("Sessions() error for missing directory: %v; want nil", err)
	}
	if len(summaries) != 0 {
		t.Errorf("Sessions() returned %d summaries for missing dir; want 0", len(summaries))
	}
}

// TestSessions_EmptyDirectory asserts that Sessions() returns an empty slice
// when the project directory exists but has no .jsonl files.
func TestSessions_EmptyDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	projectDir := filepath.Join(dir, "-empty-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	src := jsonl.NewSource(dir)
	summaries, err := src.Sessions(context.Background(), "/empty/project")
	if err != nil {
		t.Fatalf("Sessions() error for empty directory: %v; want nil", err)
	}
	if len(summaries) != 0 {
		t.Errorf("Sessions() returned %d summaries for empty dir; want 0", len(summaries))
	}
}

// TestAllProjectSessions_CrossProject verifies that AllProjectSessions enumerates
// sessions from multiple project directories and returns them ordered by
// LastActivity descending.
//
// Layout:
//
//	dir/
//	  -project-alpha/   session-a1.jsonl (mtime: 3h ago)
//	                    session-a2.jsonl (mtime: 1h ago)
//	  -project-beta/    session-b1.jsonl (mtime: 5h ago)
//	  -project-gamma/   session-g1.jsonl (mtime: 2h ago)
//
// Expected order: a2 (1h), g1 (2h), a1 (3h), b1 (5h).
// maxResults=0 means no cap.
func TestAllProjectSessions_CrossProject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)

	type sessionSpec struct {
		projectDir string
		sessionID  string
		mtime      time.Time
	}

	specs := []sessionSpec{
		{"-project-alpha", "session-a1", now.Add(-3 * time.Hour)},
		{"-project-alpha", "session-a2", now.Add(-1 * time.Hour)},
		{"-project-beta", "session-b1", now.Add(-5 * time.Hour)},
		{"-project-gamma", "session-g1", now.Add(-2 * time.Hour)},
	}

	for _, sp := range specs {
		projDir := filepath.Join(dir, sp.projectDir)
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			t.Fatal(err)
		}
		fpath := filepath.Join(projDir, sp.sessionID+".jsonl")
		if err := os.WriteFile(fpath, []byte(`{"type":"user","uuid":"x"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(fpath, sp.mtime, sp.mtime); err != nil {
			t.Fatal(err)
		}
	}

	src := jsonl.NewSource(dir)
	refs, err := src.AllProjectSessions(context.Background(), 0)
	if err != nil {
		t.Fatalf("AllProjectSessions() error: %v", err)
	}

	if len(refs) != 4 {
		t.Fatalf("AllProjectSessions() returned %d refs; want 4", len(refs))
	}

	// Verify descending order by LastActivity.
	for i := 1; i < len(refs); i++ {
		if refs[i].LastActivity.After(refs[i-1].LastActivity) {
			t.Errorf("refs[%d].LastActivity (%v) is after refs[%d].LastActivity (%v); want descending order",
				i, refs[i].LastActivity, i-1, refs[i-1].LastActivity)
		}
	}

	// Verify expected order: a2, g1, a1, b1.
	wantOrder := []session.ID{"session-a2", "session-g1", "session-a1", "session-b1"}
	for i, want := range wantOrder {
		if refs[i].SessionID != want {
			t.Errorf("refs[%d].SessionID = %q; want %q", i, refs[i].SessionID, want)
		}
	}

	// Verify Path is an absolute path to the .jsonl file.
	for i, ref := range refs {
		if ref.Path == "" {
			t.Errorf("refs[%d].Path is empty; want absolute path", i)
		}
		if !filepath.IsAbs(ref.Path) {
			t.Errorf("refs[%d].Path = %q; want absolute path", i, ref.Path)
		}
	}

	// Verify ProjectEncodedDir is set.
	for i, ref := range refs {
		if ref.ProjectEncodedDir == "" {
			t.Errorf("refs[%d].ProjectEncodedDir is empty", i)
		}
	}
}

// TestAllProjectSessions_MaxResults verifies that maxResults caps the returned slice.
func TestAllProjectSessions_MaxResults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	projDir := filepath.Join(dir, "-project-x")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create 5 session files.
	base := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		fpath := filepath.Join(projDir, fmt.Sprintf("sess-%d.jsonl", i))
		if err := os.WriteFile(fpath, []byte(`{"type":"user"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mtime := base.Add(-time.Duration(i) * time.Hour)
		if err := os.Chtimes(fpath, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	src := jsonl.NewSource(dir)

	// Ask for at most 3 results.
	refs, err := src.AllProjectSessions(context.Background(), 3)
	if err != nil {
		t.Fatalf("AllProjectSessions() error: %v", err)
	}
	if len(refs) != 3 {
		t.Errorf("AllProjectSessions(maxResults=3) returned %d refs; want 3", len(refs))
	}
}

// TestAllProjectSessions_MissingBaseDir verifies that a missing base directory
// returns an empty slice (no error).
func TestAllProjectSessions_MissingBaseDir(t *testing.T) {
	t.Parallel()

	src := jsonl.NewSource("/does/not/exist/at/all")
	refs, err := src.AllProjectSessions(context.Background(), 0)
	if err != nil {
		t.Fatalf("AllProjectSessions() error for missing base dir: %v; want nil", err)
	}
	if len(refs) != 0 {
		t.Errorf("AllProjectSessions() returned %d refs for missing base dir; want 0", len(refs))
	}
}
