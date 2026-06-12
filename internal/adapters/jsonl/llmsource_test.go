package jsonl_test

import (
	"context"
	"testing"

	"github.com/Systemartis/clyde/internal/adapters/jsonl"
	"github.com/Systemartis/clyde/internal/ports"
)

// TestLLMSourcePort verifies that *jsonl.Source fully satisfies the
// ports.LLMSource interface at compile time and that the implementation
// returns sensible values for the new methods.
func TestLLMSourcePort(t *testing.T) {
	t.Parallel()

	// Compile-time interface compliance check.
	// If jsonl.Source stops implementing LLMSource, this assignment fails to compile.
	var _ ports.LLMSource = (*jsonl.Source)(nil)

	src := jsonl.NewSource(t.TempDir()) // empty dir is safe for Name/PlanLimits

	// Name must return "claude-code" for the jsonl adapter.
	if got := src.Name(); got != "claude-code" {
		t.Errorf("Name() = %q, want %q", got, "claude-code")
	}

	// PlanLimits must return nil for claude-code (limits not detectable from JSONL).
	if limits := src.PlanLimits(context.Background()); limits != nil {
		t.Errorf("PlanLimits() = %+v, want nil", limits)
	}
}
