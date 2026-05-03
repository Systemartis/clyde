// Package project defines the Project domain entity.
//
// A Project is identified by its absolute working directory path. All Sessions
// discovered for a Project share that Project's working directory as their
// origin context. The JSONL adapter encodes the absolute path to locate the
// corresponding ~/.claude/projects/<encoded>/ directory.
package project

import "strings"

// Project is identified by its absolute working directory path. It is an
// immutable value — CWD never changes after construction.
type Project struct {
	cwd string
}

// New constructs a Project for the given absolute working directory path.
// It panics if cwd is not an absolute path (does not start with '/').
// Failing fast on a relative path prevents silently creating a Project that
// matches nothing in the filesystem.
func New(cwd string) Project {
	if !strings.HasPrefix(cwd, "/") {
		panic("project.New: cwd must be an absolute path, got: " + cwd)
	}
	return Project{cwd: cwd}
}

// CWD returns the absolute working directory path that identifies this Project.
func (p Project) CWD() string {
	return p.cwd
}
