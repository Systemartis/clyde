// Package hookserver provides a tiny localhost HTTP server that receives
// PreToolUse hook calls from the claude CLI and exposes them as a channel
// of HookEvents. Each event blocks the HTTP request until the caller sends
// a HookResponse through the event's ResponseCh, allowing the TUI to approve
// or deny tool invocations interactively.
//
// Usage:
//
//	srv, err := hookserver.New()
//	if err != nil { ... }
//	go srv.Start(ctx)
//	fmt.Printf("hook server listening on port %d\n", srv.Port())
//
//	for evt := range srv.Events() {
//	    // inspect evt, then:
//	    evt.ResponseCh <- hookserver.HookResponse{Allow: true}
//	    // OR close(evt.ResponseCh) to allow (same as Allow:true)
//	}
package hookserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// hookBodyCap bounds the size of an incoming hook payload. Real hook
// requests are tiny (a few hundred bytes); 64 KiB is generous headroom.
// Without a cap a malicious local process could stream gigabytes into
// json.Decoder and OOM the TUI.
const hookBodyCap int64 = 64 << 10

// hookTokenBytes is the random byte length of the per-process auth
// token. 32 bytes (256 bits) is overkill for a localhost listener but
// has no real cost and brings us into "obviously secure" territory.
const hookTokenBytes = 32

// HookEvent represents a single PreToolUse hook invocation from the claude CLI.
// The HTTP request that delivered this event is blocked until ResponseCh
// receives a value or is closed.
type HookEvent struct {
	// Type is the hook type string, e.g. "PreToolUse".
	Type string

	// Tool is the tool name, e.g. "Bash", "Edit", "Read".
	Tool string

	// Args holds the tool arguments as decoded from the request JSON.
	Args map[string]any

	// Cwd is the working directory context for the tool call (may be empty).
	Cwd string

	// Timestamp is when the event was received.
	Timestamp time.Time

	// ResponseCh is written to exactly once by the consumer to reply.
	// Sending a HookResponse with Allow=false and a non-empty Reason
	// causes the HTTP response to deny the tool call. Sending Allow=true
	// (or closing the channel) approves it. The server drains the channel
	// regardless so callers must not leave it un-answered — they'll block
	// the corresponding HTTP request indefinitely otherwise.
	ResponseCh chan HookResponse
}

// HookResponse is the reply the consumer sends back through HookEvent.ResponseCh.
type HookResponse struct {
	// Allow indicates whether the tool call should proceed.
	// When false the server returns an HTTP 200 with a JSON deny payload.
	Allow bool

	// Reason is an optional human-readable string included in the deny response.
	// Ignored when Allow is true.
	Reason string
}

// Server is the localhost HTTP hook server. Construct with New; start with Start.
type Server struct {
	listener net.Listener
	port     int
	token    string
	events   chan HookEvent
	srv      *http.Server
	once     sync.Once
}

// New creates a new Server bound to 127.0.0.1:0 (OS-assigned free port)
// with a fresh per-process auth token. Returns an error if the listener
// cannot be opened or the random token cannot be generated.
func New() (*Server, error) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("hookserver: listen: %w", err)
	}

	tokenBytes := make([]byte, hookTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("hookserver: generate token: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port

	s := &Server{
		listener: ln,
		port:     port,
		token:    hex.EncodeToString(tokenBytes),
		events:   make(chan HookEvent, 8),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/hook", s.handleHook)

	s.srv = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute, // long: we block until user answers
		IdleTimeout:  120 * time.Second,
	}

	return s, nil
}

// Port returns the port the server is bound to.
// Only valid after New() succeeds.
func (s *Server) Port() int {
	return s.port
}

// Token returns the per-process auth token clients must present to
// post hook events. Tells the user how to wire the URL into Claude
// Code's hook config; never include this in any persistent log.
func (s *Server) Token() string {
	return s.token
}

// URL returns the full callback URL (including the auth token) that
// Claude Code's hook config should point at.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/hook?t=%s", s.port, s.token)
}

// Events returns the read-only channel on which incoming HookEvents arrive.
// Consumers must drain this channel and reply via each event's ResponseCh,
// otherwise the corresponding HTTP request (and the claude CLI) will block.
func (s *Server) Events() <-chan HookEvent {
	return s.events
}

// Start begins serving requests. It blocks until ctx is canceled, at which
// point it performs a graceful shutdown and closes the events channel.
// Start is safe to call in a goroutine. It must only be called once.
func (s *Server) Start(ctx context.Context) error {
	// Buffered so the goroutine can deposit its result and exit even if
	// nobody is yet receiving. The send-into-channel happens-before the
	// receive below, which gives Start the synchronization edge a plain
	// stack-allocated error variable lacks.
	serveErrCh := make(chan error, 1)

	go func() {
		if err := s.srv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			serveErrCh <- err
			return
		}
		serveErrCh <- nil
	}()

	<-ctx.Done()

	// Graceful shutdown — give in-flight requests up to 5s to complete.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.once.Do(func() {
		_ = s.srv.Shutdown(shutdownCtx)
		close(s.events)
	})

	return <-serveErrCh
}

// hookRequest is the JSON body expected from the claude CLI hook call.
type hookRequest struct {
	HookType string         `json:"hook_type"`
	Tool     string         `json:"tool_name"`
	Args     map[string]any `json:"tool_input"`
	Cwd      string         `json:"cwd"`
}

// hookDenyResponse is sent back to claude CLI when the user denies the call.
type hookDenyResponse struct {
	Decision string `json:"decision"` // "block"
	Reason   string `json:"reason,omitempty"`
}

// hookAllowResponse is sent back when the user approves the call.
type hookAllowResponse struct {
	Decision string `json:"decision"` // "approve"
}

// handleHook is the HTTP handler for POST /hook.
func (s *Server) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Bound the request body — without this a local malicious process
	// could stream gigabytes into json.Decoder and OOM clyde.
	r.Body = http.MaxBytesReader(w, r.Body, hookBodyCap)
	var req hookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	evt := HookEvent{
		Type:       req.HookType,
		Tool:       req.Tool,
		Args:       req.Args,
		Cwd:        req.Cwd,
		Timestamp:  time.Now(),
		ResponseCh: make(chan HookResponse, 1),
	}

	// Deliver to consumer. If the events channel is full or has no consumer,
	// fail closed: deny the call rather than approving silently. Auto-allow
	// would let any local process push hook calls past a hung TUI and harvest
	// approvals the user never granted; deny forces a re-prompt instead.
	select {
	case s.events <- evt:
	default:
		writeDeny(w, "clyde busy — re-run the tool to retry")
		return
	}

	// Wait for the consumer's decision.
	resp, ok := <-evt.ResponseCh
	if !ok || resp.Allow {
		writeAllow(w)
		return
	}
	writeDeny(w, resp.Reason)
}

// authorized accepts a request whose token matches s.token, supplied
// either as the `t` query parameter or as a Bearer Authorization
// header. Comparison is constant-time to prevent token-shape leaks
// via timing.
//
// Note: this is defense against another local process on the same
// machine, NOT against a remote attacker — the listener is loopback
// only. The threat model is "another unprivileged process on the user's
// box should not be able to spoof PreToolUse notifications and trick
// the user into approving an attacker-controlled tool call".
func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	got := r.URL.Query().Get("t")
	if got == "" {
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, prefix) {
			got = auth[len(prefix):]
		}
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

// writeAllow writes an approve JSON response.
func writeAllow(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(hookAllowResponse{Decision: "approve"})
}

// writeDeny writes a block JSON response with an optional reason.
func writeDeny(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(hookDenyResponse{Decision: "block", Reason: reason})
}
