package nextmoca

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientSelectSendsContractAndValidatesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["request_id"] != "request-1" {
			t.Fatalf("request id = %v", body["request_id"])
		}
		if got := body["security_mode"]; got != "quarantine" {
			t.Fatalf("security mode = %v", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Needlepath-Selection-Id", "selection-1")
		_, _ = writer.Write([]byte(`{"request_id":"request-1","outcome":"selected","fallback_used":false,"rendered_context":"selected","tokens_before":100,"tokens_after":20,"tokens_saved":80,"engine_latency_ms":7.5,"selected":[{"record_id":"message-1","excerpt":"selected"}]}`))
	}))
	defer server.Close()

	client := NewHTTPClient(Config{Endpoint: server.URL, APIKey: "test-key", Mode: "quarantine", MaxContextTokens: 8192, CapacityTokens: 128000})
	response, err := client.Select(context.Background(), Request{
		RequestID:  "request-1",
		TaskPrompt: "What is the code?",
		Records:    []Record{{ID: "message-1", Kind: "tool_result", Source: "tool", SourceKind: "tool", Text: "code H7K2"}},
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if response.SelectionID != "selection-1" || response.TokensSaved != 80 || len(response.Selected) != 1 {
		t.Fatalf("response = %#v", response)
	}
}

func TestHTTPClientRejectsMismatchedResponseRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"request_id":"wrong","outcome":"selected","fallback_used":false,"rendered_context":"selected","tokens_before":1,"tokens_after":1,"tokens_saved":0,"engine_latency_ms":1,"selected":[]}`))
	}))
	defer server.Close()

	client := NewHTTPClient(Config{Endpoint: server.URL, APIKey: "test-key", Mode: "active", MaxContextTokens: 1, CapacityTokens: 1})
	_, err := client.Select(context.Background(), Request{RequestID: "request-1", TaskPrompt: "task"})
	if err == nil {
		t.Fatal("Select() error = nil, want contract error")
	}
}
