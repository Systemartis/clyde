// Package project_test contains table-driven tests for the Project domain type.
package project_test

import (
	"testing"

	"github.com/Systemartis/clyde/internal/domain/project"
)

// TestProjectCWD verifies that a Project created with a given working directory
// returns that exact path from CWD().
func TestProjectCWD(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cwd  string
		want string
	}{
		{
			name: "unix absolute path",
			cwd:  "/Users/foo/bar",
			want: "/Users/foo/bar",
		},
		{
			name: "deep nested path",
			cwd:  "/home/user/work/projects/myapp",
			want: "/home/user/work/projects/myapp",
		},
		{
			name: "root path",
			cwd:  "/",
			want: "/",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := project.New(tc.cwd)
			if got := p.CWD(); got != tc.want {
				t.Errorf("Project.CWD() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestProjectRelativePathRejected verifies that New panics (or returns an
// appropriate zero/invalid state) when given a relative path.
// Per spec: "A Project is identified by its ABSOLUTE working directory path."
// Design choice: panic on relative path — failing fast is better than silently
// producing a corrupt Project that would match nothing in the filesystem.
func TestProjectRelativePathRejected(t *testing.T) {
	t.Parallel()

	relativePaths := []string{
		"relative/path",
		"./also/relative",
		"../up/and/over",
	}

	for _, p := range relativePaths {
		p := p
		t.Run(p, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if r := recover(); r == nil {
					t.Errorf("New(%q) did not panic on relative path; expected panic", p)
				}
			}()

			project.New(p) //nolint:errcheck // should panic
		})
	}
}
