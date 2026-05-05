package hookserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/adapters/hookserver"
)

// newTestServer starts a Server and returns it with a cancel function.
// The cancel function stops the server; the test must call it.
func newTestServer(t *testing.T) (*hookserver.Server, context.CancelFunc) {
	t.Helper()
	srv, err := hookserver.New()
	if err != nil {
		t.Fatalf("hookserver.New() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = srv.Start(ctx)
	}()

	// Give the server a moment to accept connections.
	time.Sleep(10 * time.Millisecond)

	return srv, cancel
}

// postHook sends a POST /hook request to the given server's authenticated
// URL and returns the decoded JSON decision string ("approve" or "block").
func postHook(t *testing.T, srv *hookserver.Server, body any) string {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal payload: %v", err)
	}

	resp, err := http.Post(srv.URL(), "application/json", bytes.NewReader(payload)) //nolint:noctx
	if err != nil {
		t.Fatalf("POST /hook: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	decision, _ := result["decision"].(string)
	return decision
}

// TestNewBindsPort verifies that New() opens a listener and Port() returns
// a non-zero port number.
func TestNewBindsPort(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	if srv.Port() == 0 {
		t.Error("Server.Port() should be non-zero after New()")
	}
}

// TestPostArrivesOnEventChannel verifies that a POST /hook causes a HookEvent
// to arrive on Events() with the correct fields populated.
func TestPostArrivesOnEventChannel(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	reqBody := map[string]any{
		"hook_type":  "PreToolUse",
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "ls -la"},
		"cwd":        "/tmp/proj",
	}

	// Fire the request in background — it will block until we respond.
	resultCh := make(chan string, 1)
	go func() {
		resultCh <- postHook(t, srv, reqBody)
	}()

	// Receive the event.
	var evt hookserver.HookEvent
	select {
	case evt = <-srv.Events():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for HookEvent on channel")
	}

	// Verify fields.
	if evt.Type != "PreToolUse" {
		t.Errorf("evt.Type = %q, want %q", evt.Type, "PreToolUse")
	}
	if evt.Tool != "Bash" {
		t.Errorf("evt.Tool = %q, want %q", evt.Tool, "Bash")
	}
	if evt.Cwd != "/tmp/proj" {
		t.Errorf("evt.Cwd = %q, want %q", evt.Cwd, "/tmp/proj")
	}
	if evt.Args == nil {
		t.Error("evt.Args should be non-nil")
	} else if cmd, _ := evt.Args["command"].(string); cmd != "ls -la" {
		t.Errorf("evt.Args[command] = %q, want %q", cmd, "ls -la")
	}
	if evt.Timestamp.IsZero() {
		t.Error("evt.Timestamp should be set")
	}

	// Allow the tool call.
	evt.ResponseCh <- hookserver.HookResponse{Allow: true}

	// HTTP response should be "approve".
	select {
	case decision := <-resultCh:
		if decision != "approve" {
			t.Errorf("HTTP response decision = %q, want %q", decision, "approve")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for HTTP response after Allow=true")
	}
}

// TestAllowProducesApproveResponse verifies that responding with Allow=true
// causes the server to return {"decision":"approve"}.
func TestAllowProducesApproveResponse(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	body := map[string]any{
		"hook_type":  "PreToolUse",
		"tool_name":  "Edit",
		"tool_input": map[string]any{},
	}

	resultCh := make(chan string, 1)
	go func() {
		resultCh <- postHook(t, srv, body)
	}()

	evt := <-srv.Events()
	evt.ResponseCh <- hookserver.HookResponse{Allow: true}

	decision := <-resultCh
	if decision != "approve" {
		t.Errorf("Allow=true: decision = %q, want approve", decision)
	}
}

// TestDenyProducesBlockResponse verifies that responding with Allow=false
// causes the server to return {"decision":"block","reason":"..."}.
func TestDenyProducesBlockResponse(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	body := map[string]any{
		"hook_type":  "PreToolUse",
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "rm -rf /"},
	}

	resultCh := make(chan string, 1)
	go func() {
		resultCh <- postHook(t, srv, body)
	}()

	evt := <-srv.Events()
	evt.ResponseCh <- hookserver.HookResponse{Allow: false, Reason: "denied by user"}

	decision := <-resultCh
	if decision != "block" {
		t.Errorf("Allow=false: decision = %q, want block", decision)
	}
}

// TestMethodNotAllowedOnGet verifies that GET /hook returns 405.
func TestMethodNotAllowedOnGet(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	resp, err := http.Get(srv.URL()) //nolint:noctx
	if err != nil {
		t.Fatalf("GET /hook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /hook status = %d, want 405", resp.StatusCode)
	}
}

// TestBadRequestOnMalformedJSON verifies that malformed JSON returns 400.
func TestBadRequestOnMalformedJSON(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	resp, err := http.Post(srv.URL(), "application/json", bytes.NewBufferString("{not-json")) //nolint:noctx
	if err != nil {
		t.Fatalf("POST /hook (bad json): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed JSON: status = %d, want 400", resp.StatusCode)
	}
}

// TestUnauthorizedWithoutToken verifies that a POST to /hook without
// the auth token is rejected with 401. This is the security-audit fix
// for the unauthenticated localhost listener — a malicious local
// process must NOT be able to spoof PreToolUse notifications.
func TestUnauthorizedWithoutToken(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/hook", srv.Port())
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(`{"hook_type":"PreToolUse"}`)) //nolint:noctx
	if err != nil {
		t.Fatalf("POST /hook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", resp.StatusCode)
	}
}

// TestUnauthorizedWithWrongToken verifies that a wrong token is
// rejected. Pairs with TestUnauthorizedWithoutToken.
func TestUnauthorizedWithWrongToken(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/hook?t=not-the-real-token", srv.Port())
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(`{"hook_type":"PreToolUse"}`)) //nolint:noctx
	if err != nil {
		t.Fatalf("POST /hook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", resp.StatusCode)
	}
}

// TestAuthorizationHeaderAccepted verifies the Bearer-header path so
// callers that prefer header-based auth (cleaner logs) still work.
func TestAuthorizationHeaderAccepted(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	body := map[string]any{
		"hook_type":  "PreToolUse",
		"tool_name":  "Read",
		"tool_input": map[string]any{},
	}
	payload, _ := json.Marshal(body)

	url := fmt.Sprintf("http://127.0.0.1:%d/hook", srv.Port())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+srv.Token())
	req.Header.Set("Content-Type", "application/json")

	respCh := make(chan int, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		defer resp.Body.Close()
		respCh <- resp.StatusCode
	}()

	evt := <-srv.Events()
	evt.ResponseCh <- hookserver.HookResponse{Allow: true}

	select {
	case status := <-respCh:
		if status != http.StatusOK {
			t.Errorf("Bearer auth: status = %d, want 200", status)
		}
	case err := <-errCh:
		t.Fatalf("request error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// TestBodyCapRejectsOversizePayload verifies the security-audit fix
// for the missing body-size cap. A 1 GB POST should NOT OOM the
// process; the server returns an error.
func TestBodyCapRejectsOversizePayload(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	// Body is 200 KiB of 'x' wrapped to look JSON-ish; cap is 64 KiB.
	huge := make([]byte, 200<<10)
	for i := range huge {
		huge[i] = 'x'
	}
	payload := append([]byte(`{"hook_type":"PreToolUse","tool_input":{"x":"`), huge...)
	payload = append(payload, []byte(`"}}`)...)

	resp, err := http.Post(srv.URL(), "application/json", bytes.NewReader(payload)) //nolint:noctx
	if err != nil {
		t.Fatalf("POST /hook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Errorf("oversize body: status = 200, want non-OK (got body cap rejection)")
	}
}

// TestShutdownClosesEventChannel verifies that canceling the context
// causes Start to return and the events channel to be closed.
func TestShutdownClosesEventChannel(t *testing.T) {
	srv, cancel := newTestServer(t)

	// Cancel immediately.
	cancel()

	// Events channel should close within a second.
	timeout := time.After(1 * time.Second)
	for {
		select {
		case _, open := <-srv.Events():
			if !open {
				return // success
			}
		case <-timeout:
			t.Fatal("events channel was not closed after context cancel")
		}
	}
}

// TestMultipleSequentialEvents verifies the server handles sequential requests
// correctly without losing events.
func TestMultipleSequentialEvents(t *testing.T) {
	srv, cancel := newTestServer(t)
	defer cancel()

	tools := []string{"Read", "Edit", "Bash"}
	for _, tool := range tools {
		body := map[string]any{
			"hook_type":  "PreToolUse",
			"tool_name":  tool,
			"tool_input": map[string]any{},
		}

		resultCh := make(chan string, 1)
		go func() {
			resultCh <- postHook(t, srv, body)
		}()

		evt := <-srv.Events()
		if evt.Tool != tool {
			t.Errorf("expected tool %q, got %q", tool, evt.Tool)
		}
		evt.ResponseCh <- hookserver.HookResponse{Allow: true}

		select {
		case d := <-resultCh:
			if d != "approve" {
				t.Errorf("tool %q: decision = %q, want approve", tool, d)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout on tool %q", tool)
		}
	}
}
