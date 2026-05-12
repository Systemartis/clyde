// Package version exposes build metadata for the `--version` flag.
//
// Values are populated either by ldflags at release-build time (preferred —
// see .goreleaser.yml) or by runtime/debug.ReadBuildInfo() as a fallback for
// `go install` users. When neither yields a version (a bare `go build`
// without VCS info), Info() reports "dev".
package version

import (
	"runtime"
	"runtime/debug"
)

// These vars are set via -ldflags "-X github.com/Systemartis/clyde/internal/version.X=Y"
// by goreleaser. A bare `go build` leaves them empty; ReadBuildInfo() then
// fills the gaps from the embedded VCS metadata produced by `-buildvcs=true`.
var (
	version = ""
	commit  = ""
	date    = ""
)

// BuildInfo carries the metadata reported by `clyde --version`.
type BuildInfo struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
}

// Info returns the resolved BuildInfo for this binary. Always safe to call;
// never returns a zero-value Version (falls back to "dev").
func Info() BuildInfo {
	info := BuildInfo{
		Version:   version,
		Commit:    commit,
		Date:      date,
		GoVersion: runtime.Version(),
	}
	if info.Version == "" || info.Commit == "" || info.Date == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if info.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
				info.Version = bi.Main.Version
			}
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					if info.Commit == "" {
						info.Commit = s.Value
					}
				case "vcs.time":
					if info.Date == "" {
						info.Date = s.Value
					}
				}
			}
		}
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	return info
}
