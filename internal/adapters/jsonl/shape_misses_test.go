package jsonl

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestShapeMisses_IncrementsOnMalformedAssistantUsage feeds a JSONL line
// whose `message` field is shaped wrongly for assistantMessage decode
// (Usage is a string instead of an object). The line still parses at the
// envelope level and an event is returned — but the inner Usage decode
// fails, which the shape-miss counter must reflect.
func TestShapeMisses_IncrementsOnMalformedAssistantUsage(t *testing.T) {
	// Counters are package-global; capture a baseline so this test can
	// run alongside others that also exercise the decode path.
	baseline := (&Source{}).ShapeMisses()

	root := t.TempDir()
	projectDir := filepath.Join(root, "-tmp-shape-miss-test")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	line := `{"type":"assistant","uuid":"u1","sessionId":"ses","timestamp":"2026-01-01T00:00:00Z","message":{"id":"m1","usage":"not-an-object"}}` + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, "ses.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	src := NewSource(root)
	if _, err := src.Events(context.Background(), "ses"); err != nil {
		t.Fatalf("Events: %v", err)
	}

	got := src.ShapeMisses()
	if got.Assistant <= baseline.Assistant {
		t.Errorf("ShapeMisses().Assistant did not increment: baseline=%d got=%d",
			baseline.Assistant, got.Assistant)
	}
}
