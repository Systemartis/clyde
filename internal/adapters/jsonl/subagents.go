package jsonl

// subagents.go — Subagent JSONL discovery and reading.
//
// Subagent files live at:
//
//	~/.claude/projects/<encoded-cwd>/<sessionID>/subagents/agent-<id>.jsonl
//
// Each subagent optionally has a meta file:
//
//	~/.claude/projects/<encoded-cwd>/<sessionID>/subagents/agent-<id>.meta.json
//
// The meta file, if present, carries {"agentType":"general-purpose","description":"..."}.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/ports"
)

// agentMeta is the shape of agent-<id>.meta.json files.
type agentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
}

// Subagents returns all subagent infos for a given parent session.
// Reads from <baseDir>/<encoded-cwd>/<sessionID>/subagents/agent-*.jsonl.
// Returns an empty slice (no error) when the subagents directory is absent.
func (s *Source) Subagents(_ context.Context, projectCWD, parentSessionID string) ([]ports.SubagentInfo, error) {
	dir := s.subagentDir(projectCWD, parentSessionID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var infos []ports.SubagentInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") || !strings.HasPrefix(name, "agent-") {
			continue
		}
		agentID := strings.TrimSuffix(name, ".jsonl")

		// Best-effort: read the meta.json side-car if it exists.
		var meta agentMeta
		metaPath := filepath.Join(dir, agentID+".meta.json")
		if raw, err := os.ReadFile(metaPath); err == nil {
			_ = json.Unmarshal(raw, &meta)
		}

		infos = append(infos, ports.SubagentInfo{
			AgentID:     agentID,
			Description: meta.Description,
			AgentType:   meta.AgentType,
		})
	}

	return infos, nil
}

// SubagentEvents returns all events from a subagent's JSONL file in file order.
// Returns an error if the file does not exist or contains a malformed line.
func (s *Source) SubagentEvents(_ context.Context, projectCWD, parentSessionID, agentID string) ([]event.Event, error) {
	path := filepath.Join(s.subagentDir(projectCWD, parentSessionID), agentID+".jsonl")
	return s.decodeFile(path)
}

// subagentDir returns the directory containing subagent JSONL files for a session.
func (s *Source) subagentDir(projectCWD, parentSessionID string) string {
	return filepath.Join(s.baseDir, encodeProjectPath(projectCWD), parentSessionID, "subagents")
}
