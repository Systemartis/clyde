package fsexplorer

import (
	"github.com/clyde-tui/clyde/internal/application/livesession"
)

// LiveSessionAdapter wraps Source and implements livesession.FileTreeSource.
type LiveSessionAdapter struct {
	src *Source
}

// NewLiveSessionAdapter returns a LiveSessionAdapter that satisfies
// livesession.FileTreeSource. Owns a fresh Source — the per-cwd cache
// is process-private (every clyde process gets its own walk cache).
func NewLiveSessionAdapter() *LiveSessionAdapter {
	return &LiveSessionAdapter{src: &Source{}}
}

// WalkToView implements livesession.FileTreeSource.
// It converts fsexplorer.Node into livesession.FileNode so the application
// layer has no dependency on the adapter package.
func (a *LiveSessionAdapter) WalkToView(cwd string) (*livesession.FileNode, error) {
	root, err := a.src.Walk(cwd)
	if err != nil {
		return nil, err
	}
	return convertNode(root), nil
}

// convertNode recursively converts a *Node into a *livesession.FileNode.
func convertNode(n *Node) *livesession.FileNode {
	if n == nil {
		return nil
	}
	out := &livesession.FileNode{
		Name:  n.Name,
		IsDir: n.IsDir,
		Path:  n.Path,
	}
	if len(n.Children) > 0 {
		out.Children = make([]*livesession.FileNode, len(n.Children))
		for i, c := range n.Children {
			out.Children[i] = convertNode(c)
		}
	}
	return out
}

// compile-time check: LiveSessionAdapter must satisfy livesession.FileTreeSource.
var _ livesession.FileTreeSource = (*LiveSessionAdapter)(nil)
