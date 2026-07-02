package hookserver

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestNew_RoutesErrorLogToSlog is the regression for the finding that
// http.Server.ErrorLog was nil, so server-level errors (accept failures,
// handler panics) printed to stderr and corrupted the Bubble Tea alt-screen.
// The server must install an ErrorLog that routes into slog.Default at Error
// level instead of writing to stderr.
//
// NOT parallel: it swaps the global slog default.
func TestNew_RoutesErrorLogToSlog(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))

	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.listener.Close() })

	if s.srv.ErrorLog == nil {
		t.Fatal("http.Server.ErrorLog is nil — server errors will corrupt the TUI alt-screen")
	}

	// Simulate what http.Server does internally on an error.
	s.srv.ErrorLog.Print("simulated accept error")

	out := buf.String()
	if !strings.Contains(out, "simulated accept error") {
		t.Fatalf("ErrorLog output did not reach the slog handler; buf=%q", out)
	}
	if !strings.Contains(out, `"level":"ERROR"`) {
		t.Fatalf("ErrorLog did not route at ERROR level; buf=%q", out)
	}
}

// TestServer_StartReturnsServeError closes the underlying listener before
// Start observes ctx cancellation. Serve must return a non-ErrServerClosed
// error, and Start must surface it back to the caller.
//
// Pre-fix the goroutine wrote into a stack-allocated error variable that
// the spawning goroutine read after ctx.Done() returned — without a
// synchronization edge, go test -race could (and did, intermittently) flag
// a write/read race AND occasionally observed nil despite the close.
func TestServer_StartReturnsServeError(t *testing.T) {
	t.Parallel()

	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := s.listener.Close(); err != nil {
		t.Fatalf("listener.Close: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(ctx) }()

	// Give Serve a tick to fail on the closed listener.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case got := <-errCh:
		if got == nil {
			t.Fatal("expected serve error after listener close, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return within 2s")
	}
}
