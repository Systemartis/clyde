package jsonl_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Systemartis/clyde/internal/adapters/jsonl"
	"github.com/Systemartis/clyde/internal/domain/event"
)

const subagentUserLine = `{"type":"user","uuid":"sub-evt-001","timestamp":"2026-04-30T08:00:00.000Z","sessionId":"sess-1","parentUuid":"","isSidechain":false,"message":{"role":"user","content":"explore the codebase"}}`

// seedSubagentDir builds <base>/<encoded-cwd>/<session>/subagents/ with the
// given files and returns the base dir. cwd "/test/project" encodes to
// "-test-project" (any non-alphanumeric → '-').
func seedSubagentDir(t *testing.T, files map[string]string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "-test-project", "sess-1", "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return base
}

// TestSubagents_DirAbsent verifies a session with no subagents directory
// reports no subagents and no error — the common case for solo sessions.
func TestSubagents_DirAbsent(t *testing.T) {
	t.Parallel()
	src := jsonl.NewSource(t.TempDir())
	infos, err := src.Subagents(context.Background(), "/test/project", "sess-1")
	if err != nil {
		t.Fatalf("Subagents: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d infos, want 0", len(infos))
	}
}

// TestSubagents_Discovery is the table for the discovery rules: agent-*.jsonl
// files become SubagentInfos; the .meta.json side-car is best-effort (absent
// or malformed meta must not fail discovery); non-matching names are ignored.
func TestSubagents_Discovery(t *testing.T) {
	t.Parallel()
	base := seedSubagentDir(t, map[string]string{
		"agent-abc.jsonl":     subagentUserLine + "\n",
		"agent-abc.meta.json": `{"agentType":"general-purpose","description":"explore the code"}`,
		"agent-def.jsonl":     subagentUserLine + "\n", // no meta side-car
		"agent-ghi.jsonl":     subagentUserLine + "\n",
		"agent-ghi.meta.json": `{not json`, // malformed meta: tolerated
		"notes.txt":           "ignored",
		"other.jsonl":         "ignored — no agent- prefix",
	})
	src := jsonl.NewSource(base)

	infos, err := src.Subagents(context.Background(), "/test/project", "sess-1")
	if err != nil {
		t.Fatalf("Subagents: %v", err)
	}
	if len(infos) != 3 {
		t.Fatalf("got %d infos, want 3: %+v", len(infos), infos)
	}

	byID := map[string]struct{ typ, desc string }{}
	for _, in := range infos {
		byID[in.AgentID] = struct{ typ, desc string }{in.AgentType, in.Description}
	}
	if got := byID["agent-abc"]; got.typ != "general-purpose" || got.desc != "explore the code" {
		t.Errorf("agent-abc meta = %+v, want general-purpose / 'explore the code'", got)
	}
	if got := byID["agent-def"]; got.typ != "" || got.desc != "" {
		t.Errorf("agent-def (no meta) = %+v, want empty meta fields", got)
	}
	if got := byID["agent-ghi"]; got.typ != "" || got.desc != "" {
		t.Errorf("agent-ghi (malformed meta) = %+v, want empty meta fields", got)
	}
}

// TestSubagentEvents_ReadsFile verifies events come back in file order, and
// that a missing agent file is an error (callers rely on it to distinguish
// "agent gone" from "agent idle").
func TestSubagentEvents_ReadsFile(t *testing.T) {
	t.Parallel()
	base := seedSubagentDir(t, map[string]string{
		"agent-abc.jsonl": subagentUserLine + "\n",
	})
	src := jsonl.NewSource(base)

	evts, err := src.SubagentEvents(context.Background(), "/test/project", "sess-1", "agent-abc")
	if err != nil {
		t.Fatalf("SubagentEvents: %v", err)
	}
	if len(evts) != 1 || evts[0].Kind != event.KindUser {
		t.Errorf("got %d events (first kind %v), want 1 user event", len(evts), evts[0].Kind)
	}

	if _, err := src.SubagentEvents(context.Background(), "/test/project", "sess-1", "agent-missing"); err == nil {
		t.Error("missing agent file must return an error")
	}
}
