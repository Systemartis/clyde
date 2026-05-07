// Package clydelog wires a structured logger (slog) for clyde.
//
// Why a dedicated package: the TUI owns stdout/stderr — writing slog output
// there would corrupt the bubbletea-rendered alt screen. clydelog instead
// writes JSON-formatted records to a file under $XDG_CACHE_HOME/clyde/, so
// users can `tail -f` it from a second pane and bug reports include real
// chronology instead of guesses.
//
// Default level is Info. CLYDE_DEBUG=1 (or any non-empty value) raises it
// to Debug — adapters should log at Debug for per-tick diagnostic spew and
// at Info+ for things worth surfacing without the env var.
//
// File path resolution mirrors writeHookURLFile in cmd/clyde/main.go:
//
//	$CLYDE_LOG_FILE > $XDG_CACHE_HOME/clyde/clyde.log > ~/.cache/clyde/clyde.log
//
// If the file cannot be opened (no $HOME, read-only filesystem, etc.) the
// logger is wired to io.Discard so the rest of clyde keeps running. The
// returned (path, err) lets the caller report the situation to the user.
package clydelog

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Setup constructs an slog.Logger writing JSON records to clyde's per-user
// log file and installs it as slog.Default. The returned closer must be
// invoked at shutdown to flush + close the underlying file. The returned
// path is the resolved on-disk location (empty string if the logger fell
// back to io.Discard).
func Setup() (path string, closer io.Closer, err error) {
	level := slog.LevelInfo
	if os.Getenv("CLYDE_DEBUG") != "" {
		level = slog.LevelDebug
	}

	path, err = resolveLogPath()
	if err != nil {
		// No usable home/cache dir — discard logs but keep clyde alive.
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: level})))
		return "", noopCloser{}, fmt.Errorf("clydelog: resolve path: %w", err)
	}

	if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr != nil {
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: level})))
		return "", noopCloser{}, fmt.Errorf("clydelog: mkdir: %w", mkErr)
	}

	// Append, create on first run, owner-readable only.
	// G304: path comes from $XDG_CACHE_HOME (env-controlled by the user's
	// own shell); writing the user's own log file is the entire purpose.
	f, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // see comment
	if openErr != nil {
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: level})))
		return "", noopCloser{}, fmt.Errorf("clydelog: open %s: %w", path, openErr)
	}

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: level,
		// AddSource=false: bubbletea's per-tick logs would balloon the file
		// with go-runtime call sites that the user doesn't need. Adapters
		// that want a source should use slog.LogAttrs with an explicit "src".
	})
	slog.SetDefault(slog.New(handler))

	slog.Info("clydelog: ready",
		slog.String("path", path),
		slog.String("level", level.String()),
		slog.Bool("debug_env", os.Getenv("CLYDE_DEBUG") != ""),
	)

	return path, f, nil
}

// resolveLogPath returns the absolute path to the log file. Resolution order:
//   - $CLYDE_LOG_FILE (escape hatch for tests / unusual layouts)
//   - $XDG_CACHE_HOME/clyde/clyde.log
//   - ~/.cache/clyde/clyde.log
func resolveLogPath() (string, error) {
	if p := os.Getenv("CLYDE_LOG_FILE"); p != "" {
		return p, nil
	}
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.New("no $HOME and no $XDG_CACHE_HOME")
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheHome, "clyde", "clyde.log"), nil
}

// noopCloser is returned when the logger falls back to io.Discard so callers
// can defer closer.Close() unconditionally.
type noopCloser struct{}

func (noopCloser) Close() error { return nil }
