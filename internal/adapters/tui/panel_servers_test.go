package tui

import (
	"testing"
)

// TestAllocateServerBudget_FitsBoth verifies the easy case where everything fits.
func TestAllocateServerBudget_FitsBoth(t *testing.T) {
	t.Parallel()
	mcp, lsp := allocateServerBudget(3, 2, 12)
	if mcp != 3 || lsp != 2 {
		t.Errorf("allocateServerBudget(3, 2, 12) = (%d, %d), want (3, 2)", mcp, lsp)
	}
}

// TestAllocateServerBudget_OnlyOneSection verifies a single populated section
// gets the full item budget (minus its own header + trailer slack).
func TestAllocateServerBudget_OnlyOneSection(t *testing.T) {
	t.Parallel()
	mcp, lsp := allocateServerBudget(20, 0, 8)
	if lsp != 0 {
		t.Errorf("lsp = %d, want 0 (no LSPs)", lsp)
	}
	// 8 inner - 1 mcp header - 2 trailers = 5 items max
	if mcp != 5 {
		t.Errorf("mcp = %d, want 5 (capped at maxItems)", mcp)
	}
}

// TestAllocateServerBudget_Proportional verifies the proportional split when
// both sections need to truncate.
func TestAllocateServerBudget_Proportional(t *testing.T) {
	t.Parallel()
	// innerH 10 → chrome 3 (mcp+lsp+sep), trailers 2 → maxItems = 5.
	// Total 8+2=10 items, ratio 8:2 → mcp = 5*8/10 = 4, lsp = 5-4 = 1.
	mcp, lsp := allocateServerBudget(8, 2, 10)
	if mcp+lsp > 5 {
		t.Errorf("mcp+lsp = %d, want ≤ 5", mcp+lsp)
	}
	if mcp <= lsp {
		t.Errorf("mcp (%d) should be greater than lsp (%d) per ratio", mcp, lsp)
	}
}

// TestAllocateServerBudget_ReflowsSlack verifies that when one section has
// fewer items than its proportional share, the excess goes to the other.
func TestAllocateServerBudget_ReflowsSlack(t *testing.T) {
	t.Parallel()
	// innerH 8 → chrome 3, trailers 2 → maxItems = 3.
	// LSP only has 1 item, even though its share is larger by ratio. The
	// remaining slot should go to MCP.
	mcp, lsp := allocateServerBudget(10, 1, 8)
	if lsp != 1 {
		t.Errorf("lsp = %d, want 1 (only 1 available)", lsp)
	}
	if mcp != 2 {
		t.Errorf("mcp = %d, want 2 (3 budget - 1 lsp)", mcp)
	}
}

// TestCleanMCPName_StripsAtSuffix verifies the registry-source suffix is gone.
func TestCleanMCPName_StripsAtSuffix(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"engram@engram":                             "engram",
		"superpowers@claude-plugins-official":       "superpowers",
		"claude-code-setup@claude-plugins-official": "claude-code-setup",
		"plain":        "plain",
		"":             "",
		"@only-source": "",
	}
	for input, want := range cases {
		if got := cleanMCPName(input); got != want {
			t.Errorf("cleanMCPName(%q) = %q, want %q", input, got, want)
		}
	}
}
