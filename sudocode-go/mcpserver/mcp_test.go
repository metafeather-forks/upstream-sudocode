package mcpserver

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestRegisterTools(t *testing.T) {
	s := server.NewMCPServer("test", "1.0.0",
		server.WithToolCapabilities(true),
	)
	RegisterTools(s)

	// Verify all 11 tools are registered by listing them
	tools := MCPTools()
	if len(tools) != 11 {
		t.Fatalf("expected 11 tools, got %d", len(tools))
	}

	expectedNames := []string{
		"ready",
		"list_issues",
		"show_issue",
		"upsert_issue",
		"list_specs",
		"show_spec",
		"upsert_spec",
		"link",
		"add_reference",
		"add_feedback",
		"get_project_id",
	}

	for i, tool := range tools {
		if tool.Name != expectedNames[i] {
			t.Errorf("tool %d: expected name %q, got %q", i, expectedNames[i], tool.Name)
		}
	}
}

func TestToolDefinitions(t *testing.T) {
	tools := MCPTools()

	// Verify each tool has a description
	for _, tool := range tools {
		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
	}

	// Verify required parameters exist on key tools
	tests := []struct {
		name           string
		requiredParams []string
	}{
		{"ready", []string{"project_id"}},
		{"show_issue", []string{"project_id", "issue_id"}},
		{"show_spec", []string{"project_id", "spec_id"}},
		{"link", []string{"project_id", "from_id", "to_id", "type"}},
	}

	toolMap := make(map[string]mcp.Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	for _, tt := range tests {
		tool, ok := toolMap[tt.name]
		if !ok {
			t.Fatalf("tool %q not found", tt.name)
		}
		schema := tool.InputSchema.Properties
		for _, param := range tt.requiredParams {
			if _, exists := schema[param]; !exists {
				t.Errorf("tool %q: missing required parameter %q", tt.name, param)
			}
		}
	}
}

func TestGetProjectIDHandler(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Name = "get_project_id"
	req.Params.Arguments = map[string]interface{}{
		"path": "/home/user/my-project",
	}

	result, err := handleGetProjectID(t.Context(), req)
	if err != nil {
		t.Fatalf("handleGetProjectID: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Result should contain the path-derived project ID
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
}

func TestEntityType(t *testing.T) {
	tests := []struct {
		id       string
		expected string
	}{
		{"i-abc123", "issue"},
		{"s-xyz789", "spec"},
		{"unknown", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		got := entityType(tt.id)
		if got != tt.expected {
			t.Errorf("entityType(%q) = %q, want %q", tt.id, got, tt.expected)
		}
	}
}

func TestNewMCPServer(t *testing.T) {
	s := NewMCPServer()
	if s == nil {
		t.Fatal("NewMCPServer returned nil")
	}
}

func TestWithProjectMissingID(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}

	_, err := withProject(t.Context(), req)
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}
