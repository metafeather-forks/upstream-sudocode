package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"encore.dev/beta/auth"

	localauth "encore.app/sudocode-go/auth"
	"encore.app/sudocode-go/projects"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// withProject injects the project_id from MCP tool arguments into the context
// so that the projects service auth handler can extract it.
func withProject(ctx context.Context, req mcp.CallToolRequest) (context.Context, error) {
	pid := req.GetString("project_id", "")
	if pid == "" {
		return ctx, fmt.Errorf("project_id is required")
	}
	return auth.WithContext(ctx, auth.UID(pid), &localauth.Data{ProjectID: pid}), nil
}

// stringPtr returns a pointer to s, or nil if s is empty.
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// intPtr returns a pointer to v.
func intPtr(v int) *int {
	return &v
}

// boolPtr returns a pointer to v.
func boolPtr(v bool) *bool {
	return &v
}

// toolResult marshals v to JSON and returns it as an MCP text result.
func toolResult(v interface{}) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// RegisterTools adds all 11 sudocode MCP tools to the given server.
func RegisterTools(s *server.MCPServer) {
	s.AddTool(readyTool(), handleReady)
	s.AddTool(listIssuesTool(), handleListIssues)
	s.AddTool(showIssueTool(), handleShowIssue)
	s.AddTool(upsertIssueTool(), handleUpsertIssue)
	s.AddTool(listSpecsTool(), handleListSpecs)
	s.AddTool(showSpecTool(), handleShowSpec)
	s.AddTool(upsertSpecTool(), handleUpsertSpec)
	s.AddTool(linkTool(), handleLink)
	s.AddTool(addReferenceTool(), handleAddReference)
	s.AddTool(addFeedbackTool(), handleAddFeedback)
	s.AddTool(getProjectIDTool(), handleGetProjectID)
}

// --- Tool definitions ---

func readyTool() mcp.Tool {
	return mcp.NewTool("ready",
		mcp.WithDescription("Shows current project state: ready issues (no blockers), in-progress, and blocked."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
	)
}

func listIssuesTool() mcp.Tool {
	return mcp.NewTool("list_issues",
		mcp.WithDescription("Search and filter issues by status, priority, or keyword."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
		mcp.WithString("search", mcp.Description("Search text for titles and descriptions")),
		mcp.WithString("status", mcp.Description("Filter by status: open, in_progress, blocked, needs_review, closed, wont_fix, duplicate")),
		mcp.WithNumber("priority", mcp.Description("Filter by priority (0=highest, 4=lowest)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 50)")),
		mcp.WithBoolean("archived", mcp.Description("Include archived issues")),
	)
}

func showIssueTool() mcp.Tool {
	return mcp.NewTool("show_issue",
		mcp.WithDescription("Get full details about a specific issue."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
		mcp.WithString("issue_id", mcp.Required(), mcp.Description("Issue ID (e.g. i-xxxx)")),
	)
}

func upsertIssueTool() mcp.Tool {
	return mcp.NewTool("upsert_issue",
		mcp.WithDescription("Create or update an issue."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
		mcp.WithString("issue_id", mcp.Required(), mcp.Description("Issue ID (e.g. i-xxxx)")),
		mcp.WithString("title", mcp.Description("Issue title")),
		mcp.WithString("status", mcp.Description("Status: open, in_progress, blocked, needs_review, closed, wont_fix, duplicate")),
		mcp.WithString("description", mcp.Description("Issue description/content")),
		mcp.WithNumber("priority", mcp.Description("Priority: 0 (highest) to 4 (lowest)")),
		mcp.WithString("parent", mcp.Description("Parent issue ID")),
		mcp.WithBoolean("archived", mcp.Description("Archive the issue")),
	)
}

func listSpecsTool() mcp.Tool {
	return mcp.NewTool("list_specs",
		mcp.WithDescription("Search and browse all specs in the project."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
		mcp.WithString("search", mcp.Description("Search text")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 50)")),
	)
}

func showSpecTool() mcp.Tool {
	return mcp.NewTool("show_spec",
		mcp.WithDescription("Get full details about a specific spec."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
		mcp.WithString("spec_id", mcp.Required(), mcp.Description("Spec ID (e.g. s-xxxx)")),
	)
}

func upsertSpecTool() mcp.Tool {
	return mcp.NewTool("upsert_spec",
		mcp.WithDescription("Create or update a spec."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
		mcp.WithString("spec_id", mcp.Required(), mcp.Description("Spec ID (e.g. s-xxxx)")),
		mcp.WithString("title", mcp.Description("Spec title")),
		mcp.WithString("description", mcp.Description("Spec content in markdown")),
		mcp.WithNumber("priority", mcp.Description("Priority: 0 (highest) to 4 (lowest)")),
		mcp.WithString("parent", mcp.Description("Parent spec ID")),
		mcp.WithBoolean("archived", mcp.Description("Archive the spec")),
	)
}

func linkTool() mcp.Tool {
	return mcp.NewTool("link",
		mcp.WithDescription("Create a relationship between specs and/or issues."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
		mcp.WithString("from_id", mcp.Required(), mcp.Description("Source entity ID")),
		mcp.WithString("to_id", mcp.Required(), mcp.Description("Target entity ID")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Relationship type: blocks, implements, references, depends-on, discovered-from, related")),
	)
}

func addReferenceTool() mcp.Tool {
	return mcp.NewTool("add_reference",
		mcp.WithDescription("Insert an [[ID]] reference into spec or issue content. Uses the link tool under the hood."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
		mcp.WithString("entity_id", mcp.Required(), mcp.Description("Entity whose content gets the reference")),
		mcp.WithString("reference_id", mcp.Required(), mcp.Description("Entity being referenced")),
		mcp.WithString("relationship_type", mcp.Description("Optional relationship type")),
	)
}

func addFeedbackTool() mcp.Tool {
	return mcp.NewTool("add_feedback",
		mcp.WithDescription("Provide feedback on a spec or issue. Required when closing issues that implement specs."),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("Project ID")),
		mcp.WithString("to_id", mcp.Required(), mcp.Description("Target entity receiving feedback")),
		mcp.WithString("content", mcp.Description("Feedback content in markdown")),
		mcp.WithString("type", mcp.Description("Feedback type: comment, suggestion, request")),
		mcp.WithString("issue_id", mcp.Description("Issue ID providing the feedback")),
		mcp.WithString("text", mcp.Description("Text to anchor feedback to")),
		mcp.WithNumber("line", mcp.Description("Line number to anchor feedback")),
	)
}

func getProjectIDTool() mcp.Tool {
	return mcp.NewTool("get_project_id",
		mcp.WithDescription("Get the deterministic project ID for a given path."),
		mcp.WithString("path", mcp.Description("Absolute or relative path")),
	)
}

// --- Tool handlers ---

func handleReady(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := projects.Ready(ctx)
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleListIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	params := &projects.ListIssuesParams{
		Search: req.GetString("search", ""),
		Status: req.GetString("status", ""),
		Limit:  req.GetInt("limit", 0),
	}
	args := req.GetArguments()
	if v, ok := args["priority"]; ok {
		if f, ok := v.(float64); ok {
			params.Priority = int(f)
		}
	}
	if v, ok := args["archived"]; ok {
		if b, ok := v.(bool); ok {
			params.Archived = b
		}
	}
	resp, err := projects.ListIssues(ctx, params)
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleShowIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	id := req.GetString("issue_id", "")
	if id == "" {
		return nil, fmt.Errorf("issue_id is required")
	}
	resp, err := projects.ShowIssue(ctx, &projects.ShowIssueParams{ID: id})
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleUpsertIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	id := req.GetString("issue_id", "")
	if id == "" {
		return nil, fmt.Errorf("issue_id is required")
	}
	params := &projects.UpsertIssueParams{ID: id}
	if v := req.GetString("title", ""); v != "" {
		params.Title = stringPtr(v)
	}
	if v := req.GetString("status", ""); v != "" {
		params.Status = stringPtr(v)
	}
	if v := req.GetString("description", ""); v != "" {
		params.Content = stringPtr(v)
	}
	args := req.GetArguments()
	if v, ok := args["priority"]; ok {
		if f, ok := v.(float64); ok {
			p := int(f)
			params.Priority = &p
		}
	}
	if v := req.GetString("parent", ""); v != "" {
		params.ParentID = stringPtr(v)
	}
	if v, ok := args["archived"]; ok {
		if b, ok := v.(bool); ok {
			params.Archived = boolPtr(b)
		}
	}
	// Parse tags from arguments if present
	if v, ok := args["tags"]; ok {
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					params.Tags = append(params.Tags, s)
				}
			}
		}
	}
	resp, err := projects.UpsertIssue(ctx, params)
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleListSpecs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	params := &projects.ListSpecsParams{
		Search: req.GetString("search", ""),
		Limit:  req.GetInt("limit", 0),
	}
	resp, err := projects.ListSpecs(ctx, params)
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleShowSpec(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	id := req.GetString("spec_id", "")
	if id == "" {
		return nil, fmt.Errorf("spec_id is required")
	}
	resp, err := projects.ShowSpec(ctx, &projects.ShowSpecParams{ID: id})
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleUpsertSpec(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	id := req.GetString("spec_id", "")
	if id == "" {
		return nil, fmt.Errorf("spec_id is required")
	}
	params := &projects.UpsertSpecParams{ID: id}
	if v := req.GetString("title", ""); v != "" {
		params.Title = stringPtr(v)
	}
	if v := req.GetString("description", ""); v != "" {
		params.Content = stringPtr(v)
	}
	specArgs := req.GetArguments()
	if v, ok := specArgs["priority"]; ok {
		if f, ok := v.(float64); ok {
			p := int(f)
			params.Priority = &p
		}
	}
	if v := req.GetString("parent", ""); v != "" {
		params.ParentID = stringPtr(v)
	}
	if v, ok := specArgs["archived"]; ok {
		if b, ok := v.(bool); ok {
			params.Archived = boolPtr(b)
		}
	}
	if v, ok := specArgs["tags"]; ok {
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					params.Tags = append(params.Tags, s)
				}
			}
		}
	}
	resp, err := projects.UpsertSpec(ctx, params)
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleLink(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	fromID := req.GetString("from_id", "")
	toID := req.GetString("to_id", "")
	relType := req.GetString("type", "")
	if fromID == "" || toID == "" || relType == "" {
		return nil, fmt.Errorf("from_id, to_id, and type are required")
	}
	// Infer entity types from ID prefixes
	fromType := entityType(fromID)
	toType := entityType(toID)

	resp, err := projects.CreateRelationship(ctx, &projects.CreateRelationshipParams{
		FromID:           fromID,
		FromType:         fromType,
		ToID:             toID,
		ToType:           toType,
		RelationshipType: relType,
	})
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleAddReference(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	entityID := req.GetString("entity_id", "")
	referenceID := req.GetString("reference_id", "")
	if entityID == "" || referenceID == "" {
		return nil, fmt.Errorf("entity_id and reference_id are required")
	}
	relType := req.GetString("relationship_type", "references")

	// Create a relationship as the underlying operation
	fromType := entityType(entityID)
	toType := entityType(referenceID)
	resp, err := projects.CreateRelationship(ctx, &projects.CreateRelationshipParams{
		FromID:           entityID,
		FromType:         fromType,
		ToID:             referenceID,
		ToType:           toType,
		RelationshipType: relType,
	})
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleAddFeedback(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, err := withProject(ctx, req)
	if err != nil {
		return nil, err
	}
	toID := req.GetString("to_id", "")
	if toID == "" {
		return nil, fmt.Errorf("to_id is required")
	}
	params := &projects.CreateFeedbackParams{
		ToID:         toID,
		FeedbackType: req.GetString("type", "comment"),
		Content:      req.GetString("content", ""),
	}
	if v := req.GetString("issue_id", ""); v != "" {
		params.FromID = stringPtr(v)
	}
	resp, err := projects.CreateFeedback(ctx, params)
	if err != nil {
		return nil, err
	}
	return toolResult(resp)
}

func handleGetProjectID(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// This is a local utility — generate a deterministic project ID from path.
	// For now return the path-based ID matching the registry logic.
	path := req.GetString("path", ".")
	// Simple deterministic ID: use directory name
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	name := parts[len(parts)-1]
	if name == "" {
		name = "project"
	}
	result := map[string]string{
		"path":       path,
		"project_id": name,
	}
	return toolResult(result)
}

// entityType infers "issue" or "spec" from an ID prefix.
func entityType(id string) string {
	if strings.HasPrefix(id, "i-") {
		return "issue"
	}
	if strings.HasPrefix(id, "s-") {
		return "spec"
	}
	return "unknown"
}
