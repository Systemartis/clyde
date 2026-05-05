package ports

import (
	"context"

	"github.com/Systemartis/clyde/internal/domain/event"
)

// SubagentInfo describes a single subagent that ran under a parent session.
type SubagentInfo struct {
	// AgentID is the identifier parsed from the JSONL filename, e.g.
	// "agent-a09b7f25bb46ce5bc" from "agent-a09b7f25bb46ce5bc.jsonl".
	AgentID string

	// Description is the human-readable description from the agent's meta.json file.
	// Empty if no meta.json exists.
	Description string

	// AgentType is the type of the agent (e.g. "general-purpose").
	// Empty if no meta.json exists.
	AgentType string
}

// SubagentSource is the port through which the application layer discovers and
// reads subagent JSONL files associated with a parent session.
//
// Subagent files live at:
//
//	~/.claude/projects/<encoded-cwd>/<sessionID>/subagents/agent-<id>.jsonl
//
// Implementations MUST:
//   - Return an empty slice with no error when the subagents directory is absent
//     (most sessions have no subagents).
//   - Preserve subagent events with unknown kinds as KindOpaque — never drop them.
type SubagentSource interface {
	// Subagents lists all subagent infos for the given parent session.
	// projectCWD is needed to locate the encoded project directory.
	// Returns an empty slice (no error) when no subagents exist.
	Subagents(ctx context.Context, projectCWD string, parentSessionID string) ([]SubagentInfo, error)

	// SubagentEvents returns all events from the given subagent's JSONL file
	// in file order (ascending chronological). projectCWD and parentSessionID
	// are needed to locate the file.
	SubagentEvents(ctx context.Context, projectCWD string, parentSessionID string, agentID string) ([]event.Event, error)
}
