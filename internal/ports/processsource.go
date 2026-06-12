package ports

import (
	"context"

	"github.com/Systemartis/clyde/internal/domain/session"
)

// ProcessSource is an optional port for detecting running `claude` CLI
// processes by their `--session-id` flag. Used by the livesession layer
// to mark sessions as live even when their JSONL is mtime-stale —
// `/resume`-d sessions sit idle waiting for the next user prompt and
// would otherwise drop out of the title-bar tab strip after the
// 90-second activeSessionWindow even though the process is still
// running and the user clearly wants to come back to it.
//
// Implementations MUST:
//
//   - Return ALL session IDs that appear in a live `claude` argv on
//     the host, regardless of the cwd that process was launched in.
//     The livesession layer filters down to the current project by
//     intersecting with the per-cwd session list (set membership) —
//     ProcessSource itself is cwd-agnostic so it stays trivial to
//     implement and test.
//   - Return an empty slice + nil error when no `claude` processes are
//     running. Errors are reserved for genuine failures (cannot read
//     /proc, `ps` invocation fails, etc.).
//   - Be cheap enough to call on every TUI poll (~1s cadence). A
//     single `ps` shell-out fits.
type ProcessSource interface {
	RunningClaudeSessionIDs(ctx context.Context) ([]session.ID, error)
}
