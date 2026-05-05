package tui

import (
	"github.com/Systemartis/clyde/internal/application/livesession"
)

// deriveExplorerData converts the LiveSession view's FileTree and ModifiedFiles
// into the MockData fields consumed by the explorer panel.
//
// It is only called in live mode (not demo mode).  When View.FileTree is nil
// the tree is left unchanged; when View.ModifiedFiles is nil the modified list
// is cleared (so stale data does not persist).
func deriveExplorerData(v livesession.View, d MockData) MockData {
	// ── Modified files ────────────────────────────────────────────────────────
	if v.ModifiedFiles != nil {
		d.ModifiedFiles = convertModifiedFiles(v.ModifiedFiles)
	}

	// ── File tree ─────────────────────────────────────────────────────────────
	if v.FileTree != nil {
		d.Tree = convertFileTree(v.FileTree)
	}

	return d
}

// convertModifiedFiles maps livesession.GitFileStatus to the MockData
// ModifiedFile slice used by renderExplorer.
func convertModifiedFiles(statuses []livesession.GitFileStatus) []ModifiedFile {
	out := make([]ModifiedFile, 0, len(statuses))
	for _, s := range statuses {
		mf := ModifiedFile{Path: s.Path}
		switch s.Status {
		case 'A', '?':
			mf.Mark = "+"
		default:
			mf.Mark = "M"
		}
		// Stats are not available from `git status --porcelain` alone; leave empty.
		out = append(out, mf)
	}
	return out
}

// convertFileTree converts a livesession.FileNode tree into the flat []TreeNode
// slice that buildVisibleRows expects, using DFS pre-order traversal.
func convertFileTree(root *livesession.FileNode) []TreeNode {
	if root == nil {
		return nil
	}
	var nodes []TreeNode
	flattenNode(root, "", &nodes)
	return nodes
}

// flattenNode recursively converts a FileNode subtree to TreeNode entries,
// building the indent prefix using │ characters to match the existing renderer.
func flattenNode(n *livesession.FileNode, indent string, out *[]TreeNode) {
	for _, child := range n.Children {
		node := TreeNode{
			Indent: indent,
			IsDir:  child.IsDir,
			Name:   child.Name,
		}
		if child.IsDir {
			node.Name = "▼ " + child.Name
		}
		node.FullPath = child.Path
		*out = append(*out, node)
		if child.IsDir {
			flattenNode(child, indent+"│ ", out)
		}
	}
}
