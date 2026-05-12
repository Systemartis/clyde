package hookserver

import (
	"context"
	"testing"
	"time"
)

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
