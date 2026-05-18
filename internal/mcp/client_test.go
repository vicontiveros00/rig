package mcp

import (
	"strings"
	"testing"
)

func TestParseSSEResponse(t *testing.T) {
	client := &Client{}
	client.nextID.Store(0)

	tests := []struct {
		name      string
		body      string
		requestID int64
		wantErr   bool
		wantJSON  string
	}{
		{
			name: "single event with matching ID",
			body: "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[]}}\n\n",
			requestID: 1,
			wantJSON:  `{"tools":[]}`,
		},
		{
			name: "multiple events, match second",
			body: "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"first\":true}}\n\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"second\":true}}\n\n",
			requestID: 2,
			wantJSON:  `{"second":true}`,
		},
		{
			name:      "no matching ID",
			body:      "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":99,\"result\":{}}\n\n",
			requestID: 1,
			wantErr:   true,
		},
		{
			name:      "rpc error in response",
			body:      "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"code\":-32600,\"message\":\"invalid\"}}\n\n",
			requestID: 1,
			wantErr:   true,
		},
		{
			name:      "empty body",
			body:      "",
			requestID: 1,
			wantErr:   true,
		},
		{
			name: "trailing data without blank line",
			body: "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}",
			requestID: 1,
			wantJSON:  `{"ok":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.body)
			result, err := client.parseSSEResponse(reader, tt.requestID)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := strings.TrimSpace(string(result))
			if got != tt.wantJSON {
				t.Errorf("expected %s, got %s", tt.wantJSON, got)
			}
		})
	}
}

func TestNewRequest(t *testing.T) {
	client := NewClient("http://localhost:8080", "key123", "sse")

	req := client.newRequest("tools/list", nil)
	if req.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc=2.0, got %s", req.JSONRPC)
	}
	if req.Method != "tools/list" {
		t.Errorf("expected method=tools/list, got %s", req.Method)
	}
	if req.ID == 0 {
		t.Error("expected non-zero ID")
	}

	req2 := client.newRequest("resources/list", nil)
	if req2.ID <= req.ID {
		t.Error("expected incrementing IDs")
	}
}

func TestNewClientFields(t *testing.T) {
	client := NewClient("http://example.com/mcp", "secret", "sse")
	if client.endpoint != "http://example.com/mcp" {
		t.Errorf("unexpected endpoint: %s", client.endpoint)
	}
	if client.apiKey != "secret" {
		t.Errorf("unexpected apiKey: %s", client.apiKey)
	}
	if client.transport != "sse" {
		t.Errorf("unexpected transport: %s", client.transport)
	}
	if client.connected {
		t.Error("expected not connected initially")
	}
}

func TestSessionIDTracking(t *testing.T) {
	client := NewClient("http://localhost:8080", "", "sse")
	if client.sessionID != "" {
		t.Error("expected empty session ID initially")
	}

	client.sessionID = "test-session-123"
	if client.sessionID != "test-session-123" {
		t.Errorf("expected session ID to be set, got %s", client.sessionID)
	}
}
