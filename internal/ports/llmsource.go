// Package ports — LLMSource port definition.
//
// LLMSource is the unified abstraction for any AI CLI's session data.
// The current implementation only supports Claude Code (via the jsonl adapter),
// but the interface is designed to accept Gemini CLI, Codex CLI, Kimi CLI etc.
// in future versions.
//
// Adapter locations:
//   - claude-code  → internal/adapters/jsonl (existing)
//   - gemini-cli   → internal/adapters/gemini (V21+, not yet implemented)
//   - codex        → internal/adapters/codex  (V21+, not yet implemented)
//   - kimi         → internal/adapters/kimi   (V21+, not yet implemented)
package ports

import (
	"context"

	"github.com/clyde-tui/clyde/internal/domain/event"
	"github.com/clyde-tui/clyde/internal/domain/session"
)

// LLMSource is the unified interface for reading session data from any AI CLI.
//
// Adapters MUST satisfy this interface; the composition root selects the
// adapter at startup based on the --source flag.
//
// Contract:
//   - Sessions and Events follow the same semantics as SessionSource.
//   - AllProjectSessions follows the same semantics as GlobalSessionSource.
//   - PlanLimits returns nil when the plan/limits are unknown.
//   - Name returns a short slug, e.g. "claude-code".
type LLMSource interface {
	// Name returns the CLI source name, e.g. "claude-code", "gemini-cli".
	// Used for display in the title bar and for --source flag matching.
	Name() string

	// Sessions returns sessions for the given project working directory.
	Sessions(ctx context.Context, projectCWD string) ([]session.Summary, error)

	// Events returns events for the given session in ascending chronological order.
	Events(ctx context.Context, id session.ID) ([]event.Event, error)

	// AllProjectSessions returns references to sessions across all project
	// directories, ordered by LastActivity descending.
	// maxResults caps the result count (0 = no cap, not recommended in production).
	AllProjectSessions(ctx context.Context, maxResults int) ([]GlobalSessionRef, error)

	// PlanLimits returns the user's subscription plan limits, if detectable.
	// Returns nil when unknown (the caller falls back to hiding limit indicators).
	PlanLimits(ctx context.Context) *PlanLimits
}

// PlanLimits describes the rate limits associated with a subscription plan.
type PlanLimits struct {
	// Name is the human-readable plan name, e.g. "Max ×5", "Pro", "Free".
	Name string

	// SessionLimit is the maximum tokens per 5-hour rolling window.
	// 0 means no limit is known or the plan has no token-based session cap.
	SessionLimit int64

	// WeeklyLimit is the maximum tokens per 7-day rolling window.
	// 0 means no limit is known or the plan has no weekly token cap.
	WeeklyLimit int64
}
