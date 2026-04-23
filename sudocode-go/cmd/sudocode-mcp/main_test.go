package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestInferType(t *testing.T) {
	tests := []struct {
		id       string
		expected string
	}{
		{"i-abc", "issue"},
		{"s-xyz", "spec"},
		{"x-foo", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		got := inferType(tt.id)
		if got != tt.expected {
			t.Errorf("inferType(%q) = %q, want %q", tt.id, got, tt.expected)
		}
	}
}

func TestProxyCall(t *testing.T) {
	// Create a mock HTTP server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("X-Project-ID") != "test-project" {
			t.Errorf("expected X-Project-ID header, got %q", r.Header.Get("X-Project-ID"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready_issues": []interface{}{},
		})
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Set globals
	projectID = "test-project"
	serverURL = ts.URL
	client = ts.Client()

	result, err := proxyCall(t.Context(), "/ready", nil)
	if err != nil {
		t.Fatalf("proxyCall: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["ready_issues"]; !ok {
		t.Fatal("expected ready_issues in response")
	}
}

func TestProxyCallServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	projectID = "test-project"
	serverURL = ts.URL
	client = ts.Client()

	_, err := proxyCall(t.Context(), "/ready", nil)
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestRegisterProxyTools(t *testing.T) {
	s := server.NewMCPServer("test", "1.0.0",
		server.WithToolCapabilities(true),
	)
	projectID = "test-project"
	registerProxyTools(s)

	// Verify tools are registered by calling ListTools
	msg := mustMarshalRaw(t, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	})
	resp := s.HandleMessage(t.Context(), msg)
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var result struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	expectedTools := map[string]bool{
		"ready": true, "list_issues": true, "show_issue": true,
		"upsert_issue": true, "list_specs": true, "show_spec": true,
		"upsert_spec": true, "link": true, "add_reference": true,
		"add_feedback": true, "get_project_id": true,
	}

	for _, tool := range result.Result.Tools {
		delete(expectedTools, tool.Name)
	}
	if len(expectedTools) > 0 {
		t.Fatalf("missing tools: %v", expectedTools)
	}
}

func mustMarshalRaw(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return json.RawMessage(data)
}
