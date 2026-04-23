// Command sudocode-mcp is a thin CLI shim that bridges stdio MCP transport
// (JSON-RPC over stdin/stdout) to the sudocode daemon's HTTP/SSE MCP endpoint.
//
// Usage:
//
//	sudocode-mcp --project-id <id> [--server-url <url>]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	projectID string
	serverURL string
	client    *http.Client
)

func main() {
	flag.StringVar(&projectID, "project-id", os.Getenv("SUDOCODE_PROJECT_ID"), "Project ID")
	flag.StringVar(&serverURL, "server-url", envOr("SUDOCODE_SERVER_URL", "http://localhost:4000"), "Daemon server URL")
	flag.Parse()

	if projectID == "" {
		fmt.Fprintln(os.Stderr, "error: --project-id is required (or set SUDOCODE_PROJECT_ID)")
		os.Exit(1)
	}

	client = &http.Client{}

	s := server.NewMCPServer("sudocode", "1.0.0",
		server.WithToolCapabilities(true),
	)

	registerProxyTools(s)

	if err := server.ServeStdio(s); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// proxyCall sends a JSON-RPC tool call to the daemon HTTP endpoint and returns the result.
func proxyCall(ctx context.Context, endpoint string, body interface{}) (json.RawMessage, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := serverURL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project-ID", projectID)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// makeProxyHandler creates an MCP tool handler that proxies to a daemon endpoint.
func makeProxyHandler(endpoint string, mapArgs func(req mcp.CallToolRequest) interface{}) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		body := mapArgs(req)
		result, err := proxyCall(ctx, endpoint, body)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(result)), nil
	}
}

func registerProxyTools(s *server.MCPServer) {
	// ready
	s.AddTool(
		mcp.NewTool("ready",
			mcp.WithDescription("Shows current project state: ready issues, in-progress, and blocked."),
		),
		makeProxyHandler("/ready", func(req mcp.CallToolRequest) interface{} {
			return nil
		}),
	)

	// list_issues
	s.AddTool(
		mcp.NewTool("list_issues",
			mcp.WithDescription("Search and filter issues."),
			mcp.WithString("search", mcp.Description("Search text")),
			mcp.WithString("status", mcp.Description("Filter by status")),
			mcp.WithNumber("priority", mcp.Description("Filter by priority")),
			mcp.WithNumber("limit", mcp.Description("Max results")),
			mcp.WithBoolean("archived", mcp.Description("Include archived")),
		),
		makeProxyHandler("/issues/list", func(req mcp.CallToolRequest) interface{} {
			return map[string]interface{}{
				"search":   req.GetString("search", ""),
				"status":   req.GetString("status", ""),
				"priority": req.GetInt("priority", 0),
				"limit":    req.GetInt("limit", 0),
				"archived": req.GetBool("archived", false),
			}
		}),
	)

	// show_issue
	s.AddTool(
		mcp.NewTool("show_issue",
			mcp.WithDescription("Get full details about a specific issue."),
			mcp.WithString("issue_id", mcp.Required(), mcp.Description("Issue ID")),
		),
		makeProxyHandler("/issues/show", func(req mcp.CallToolRequest) interface{} {
			return map[string]interface{}{"id": req.GetString("issue_id", "")}
		}),
	)

	// upsert_issue
	s.AddTool(
		mcp.NewTool("upsert_issue",
			mcp.WithDescription("Create or update an issue."),
			mcp.WithString("issue_id", mcp.Required(), mcp.Description("Issue ID")),
			mcp.WithString("title", mcp.Description("Issue title")),
			mcp.WithString("status", mcp.Description("Status")),
			mcp.WithString("description", mcp.Description("Issue content")),
			mcp.WithNumber("priority", mcp.Description("Priority")),
			mcp.WithString("parent", mcp.Description("Parent issue ID")),
			mcp.WithBoolean("archived", mcp.Description("Archive")),
		),
		makeProxyHandler("/issues/upsert", func(req mcp.CallToolRequest) interface{} {
			m := map[string]interface{}{"id": req.GetString("issue_id", "")}
			if v := req.GetString("title", ""); v != "" {
				m["title"] = v
			}
			if v := req.GetString("status", ""); v != "" {
				m["status"] = v
			}
			if v := req.GetString("description", ""); v != "" {
				m["content"] = v
			}
			args := req.GetArguments()
			if v, ok := args["priority"]; ok {
				m["priority"] = v
			}
			if v := req.GetString("parent", ""); v != "" {
				m["parent_id"] = v
			}
			if v, ok := args["archived"]; ok {
				m["archived"] = v
			}
			return m
		}),
	)

	// list_specs
	s.AddTool(
		mcp.NewTool("list_specs",
			mcp.WithDescription("Search and browse specs."),
			mcp.WithString("search", mcp.Description("Search text")),
			mcp.WithNumber("limit", mcp.Description("Max results")),
		),
		makeProxyHandler("/specs/list", func(req mcp.CallToolRequest) interface{} {
			return map[string]interface{}{
				"search": req.GetString("search", ""),
				"limit":  req.GetInt("limit", 0),
			}
		}),
	)

	// show_spec
	s.AddTool(
		mcp.NewTool("show_spec",
			mcp.WithDescription("Get full details about a specific spec."),
			mcp.WithString("spec_id", mcp.Required(), mcp.Description("Spec ID")),
		),
		makeProxyHandler("/specs/show", func(req mcp.CallToolRequest) interface{} {
			return map[string]interface{}{"id": req.GetString("spec_id", "")}
		}),
	)

	// upsert_spec
	s.AddTool(
		mcp.NewTool("upsert_spec",
			mcp.WithDescription("Create or update a spec."),
			mcp.WithString("spec_id", mcp.Required(), mcp.Description("Spec ID")),
			mcp.WithString("title", mcp.Description("Spec title")),
			mcp.WithString("description", mcp.Description("Spec content")),
			mcp.WithNumber("priority", mcp.Description("Priority")),
			mcp.WithString("parent", mcp.Description("Parent spec ID")),
			mcp.WithBoolean("archived", mcp.Description("Archive")),
		),
		makeProxyHandler("/specs/upsert", func(req mcp.CallToolRequest) interface{} {
			m := map[string]interface{}{"id": req.GetString("spec_id", "")}
			if v := req.GetString("title", ""); v != "" {
				m["title"] = v
			}
			if v := req.GetString("description", ""); v != "" {
				m["content"] = v
			}
			args := req.GetArguments()
			if v, ok := args["priority"]; ok {
				m["priority"] = v
			}
			if v := req.GetString("parent", ""); v != "" {
				m["parent_id"] = v
			}
			if v, ok := args["archived"]; ok {
				m["archived"] = v
			}
			return m
		}),
	)

	// link
	s.AddTool(
		mcp.NewTool("link",
			mcp.WithDescription("Create a relationship between specs and/or issues."),
			mcp.WithString("from_id", mcp.Required(), mcp.Description("Source entity ID")),
			mcp.WithString("to_id", mcp.Required(), mcp.Description("Target entity ID")),
			mcp.WithString("type", mcp.Required(), mcp.Description("Relationship type")),
		),
		makeProxyHandler("/relationships/create", func(req mcp.CallToolRequest) interface{} {
			fromID := req.GetString("from_id", "")
			toID := req.GetString("to_id", "")
			return map[string]interface{}{
				"from_id":           fromID,
				"from_type":         inferType(fromID),
				"to_id":             toID,
				"to_type":           inferType(toID),
				"relationship_type": req.GetString("type", ""),
			}
		}),
	)

	// add_reference
	s.AddTool(
		mcp.NewTool("add_reference",
			mcp.WithDescription("Insert an [[ID]] reference into content."),
			mcp.WithString("entity_id", mcp.Required(), mcp.Description("Entity whose content gets the reference")),
			mcp.WithString("reference_id", mcp.Required(), mcp.Description("Entity being referenced")),
			mcp.WithString("relationship_type", mcp.Description("Relationship type")),
		),
		makeProxyHandler("/relationships/create", func(req mcp.CallToolRequest) interface{} {
			entityID := req.GetString("entity_id", "")
			refID := req.GetString("reference_id", "")
			relType := req.GetString("relationship_type", "references")
			return map[string]interface{}{
				"from_id":           entityID,
				"from_type":         inferType(entityID),
				"to_id":             refID,
				"to_type":           inferType(refID),
				"relationship_type": relType,
			}
		}),
	)

	// add_feedback
	s.AddTool(
		mcp.NewTool("add_feedback",
			mcp.WithDescription("Provide feedback on a spec or issue."),
			mcp.WithString("to_id", mcp.Required(), mcp.Description("Target entity")),
			mcp.WithString("content", mcp.Description("Feedback content")),
			mcp.WithString("type", mcp.Description("Feedback type")),
			mcp.WithString("issue_id", mcp.Description("Issue providing feedback")),
		),
		makeProxyHandler("/feedback/create", func(req mcp.CallToolRequest) interface{} {
			m := map[string]interface{}{
				"to_id":         req.GetString("to_id", ""),
				"feedback_type": req.GetString("type", "comment"),
				"content":       req.GetString("content", ""),
			}
			if v := req.GetString("issue_id", ""); v != "" {
				m["from_id"] = v
			}
			return m
		}),
	)

	// get_project_id
	s.AddTool(
		mcp.NewTool("get_project_id",
			mcp.WithDescription("Get the deterministic project ID for a given path."),
			mcp.WithString("path", mcp.Description("Path to resolve")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Return the configured project ID
			result := map[string]string{
				"project_id": projectID,
				"path":       req.GetString("path", "."),
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

func inferType(id string) string {
	if len(id) >= 2 {
		switch id[:2] {
		case "i-":
			return "issue"
		case "s-":
			return "spec"
		}
	}
	return "unknown"
}
