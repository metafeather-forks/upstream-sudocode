package projects

import (
	"context"
	"time"

	"encore.dev/beta/errs"
)

// --- Request/Response types ---

type ListIssuesParams struct {
	Search   string `json:"search,omitempty"`
	Status   string `json:"status,omitempty"`
	Priority int    `json:"priority,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

type ListIssuesResponse struct {
	Issues []IssueResponse `json:"issues"`
}

type IssueResponse struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Content   string   `json:"content"`
	Priority  int      `json:"priority"`
	Assignee  *string  `json:"assignee,omitempty"`
	Archived  bool     `json:"archived"`
	ParentID  *string  `json:"parent_id,omitempty"`
	ClosedAt  *string  `json:"closed_at,omitempty"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type ShowIssueParams struct {
	ID string `json:"id"`
}

type UpsertIssueParams struct {
	ID       string   `json:"id"`
	Title    *string  `json:"title,omitempty"`
	Status   *string  `json:"status,omitempty"`
	Content  *string  `json:"content,omitempty"`
	Priority *int     `json:"priority,omitempty"`
	Assignee *string  `json:"assignee,omitempty"`
	ParentID *string  `json:"parent_id,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Archived *bool    `json:"archived,omitempty"`
}

type DeleteIssueParams struct {
	ID string `json:"id"`
}

// --- Endpoints ---

// ListIssues returns issues for the authenticated project.
//
//encore:api auth method=POST path=/issues/list
func ListIssues(ctx context.Context, params *ListIssuesParams) (*ListIssuesResponse, error) {
	pid := projectID(ctx)

	query := `SELECT id, title, status, content, priority, assignee, archived, parent_id, closed_at, created_at, updated_at FROM issues WHERE project_id = $1`
	args := []interface{}{pid}
	argIdx := 2

	if !params.Archived {
		query += ` AND archived = false`
	}
	if params.Status != "" {
		query += ` AND status = $` + itoa(argIdx)
		args = append(args, params.Status)
		argIdx++
	}
	if params.Search != "" {
		query += ` AND (title ILIKE '%' || $` + itoa(argIdx) + ` || '%' OR content ILIKE '%' || $` + itoa(argIdx) + ` || '%')`
		args = append(args, params.Search)
		argIdx++
	}

	query += ` ORDER BY priority ASC, created_at DESC`

	limit := 50
	if params.Limit > 0 {
		limit = params.Limit
	}
	query += ` LIMIT $` + itoa(argIdx)
	args = append(args, limit)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []IssueResponse
	for rows.Next() {
		var i IssueResponse
		var archived bool
		var parentID, assignee *string
		var closedAt *time.Time
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&i.ID, &i.Title, &i.Status, &i.Content, &i.Priority, &assignee, &archived, &parentID, &closedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		i.Archived = archived
		i.ParentID = parentID
		i.Assignee = assignee
		if closedAt != nil {
			s := closedAt.Format(time.RFC3339)
			i.ClosedAt = &s
		}
		i.CreatedAt = createdAt.Format(time.RFC3339)
		i.UpdatedAt = updatedAt.Format(time.RFC3339)
		i.Tags, err = tagsForEntity(ctx, pid, "issue", i.ID)
		if err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	if issues == nil {
		issues = []IssueResponse{}
	}
	return &ListIssuesResponse{Issues: issues}, rows.Err()
}

// ShowIssue returns a single issue by ID.
//
//encore:api auth method=POST path=/issues/show
func ShowIssue(ctx context.Context, params *ShowIssueParams) (*IssueResponse, error) {
	pid := projectID(ctx)
	var i IssueResponse
	var archived bool
	var parentID, assignee *string
	var closedAt *time.Time
	var createdAt, updatedAt time.Time

	err := db.QueryRow(ctx, `
		SELECT id, title, status, content, priority, assignee, archived, parent_id, closed_at, created_at, updated_at
		FROM issues WHERE project_id = $1 AND id = $2
	`, pid, params.ID).Scan(&i.ID, &i.Title, &i.Status, &i.Content, &i.Priority, &assignee, &archived, &parentID, &closedAt, &createdAt, &updatedAt)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, &errs.Error{Code: errs.NotFound, Message: "issue not found"}
		}
		return nil, err
	}
	i.Archived = archived
	i.ParentID = parentID
	i.Assignee = assignee
	if closedAt != nil {
		s := closedAt.Format(time.RFC3339)
		i.ClosedAt = &s
	}
	i.CreatedAt = createdAt.Format(time.RFC3339)
	i.UpdatedAt = updatedAt.Format(time.RFC3339)
	i.Tags, err = tagsForEntity(ctx, pid, "issue", i.ID)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// UpsertIssue creates or updates an issue.
//
//encore:api auth method=POST path=/issues/upsert
func UpsertIssue(ctx context.Context, params *UpsertIssueParams) (*IssueResponse, error) {
	pid := projectID(ctx)

	if params.ID == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "id is required"}
	}

	title := ""
	if params.Title != nil {
		title = *params.Title
	}
	status := "open"
	if params.Status != nil {
		status = *params.Status
	}
	content := ""
	if params.Content != nil {
		content = *params.Content
	}
	priority := 2
	if params.Priority != nil {
		priority = *params.Priority
	}
	archived := false
	if params.Archived != nil {
		archived = *params.Archived
	}

	var createdAt, updatedAt time.Time
	var closedAt *time.Time
	err := db.QueryRow(ctx, `
		INSERT INTO issues (project_id, id, title, status, content, priority, assignee, parent_id, archived)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (project_id, id) DO UPDATE SET
			title = COALESCE(NULLIF($3, ''), issues.title),
			status = $4,
			content = CASE WHEN $5 = '' THEN issues.content ELSE $5 END,
			priority = $6,
			assignee = $7,
			parent_id = $8,
			archived = $9,
			archived_at = CASE WHEN $9 AND NOT issues.archived THEN NOW() ELSE issues.archived_at END,
			closed_at = CASE WHEN $4 = 'closed' AND issues.closed_at IS NULL THEN NOW() ELSE issues.closed_at END,
			updated_at = NOW()
		RETURNING created_at, updated_at, closed_at
	`, pid, params.ID, title, status, content, priority, params.Assignee, params.ParentID, archived).Scan(&createdAt, &updatedAt, &closedAt)
	if err != nil {
		return nil, err
	}

	if params.Tags != nil {
		if err := syncTags(ctx, pid, "issue", params.ID, params.Tags); err != nil {
			return nil, err
		}
	}

	tags, err := tagsForEntity(ctx, pid, "issue", params.ID)
	if err != nil {
		return nil, err
	}

	resp := &IssueResponse{
		ID:        params.ID,
		Title:     title,
		Status:    status,
		Content:   content,
		Priority:  priority,
		Assignee:  params.Assignee,
		Archived:  archived,
		ParentID:  params.ParentID,
		Tags:      tags,
		CreatedAt: createdAt.Format(time.RFC3339),
		UpdatedAt: updatedAt.Format(time.RFC3339),
	}
	if closedAt != nil {
		s := closedAt.Format(time.RFC3339)
		resp.ClosedAt = &s
	}
	return resp, nil
}

// DeleteIssue deletes an issue by ID.
//
//encore:api auth method=POST path=/issues/delete
func DeleteIssue(ctx context.Context, params *DeleteIssueParams) error {
	pid := projectID(ctx)
	_, err := db.Exec(ctx, `DELETE FROM issues WHERE project_id = $1 AND id = $2`, pid, params.ID)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM tags WHERE project_id = $1 AND entity_type = 'issue' AND entity_id = $2`, pid, params.ID)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM relationships WHERE project_id = $1 AND (from_id = $2 OR to_id = $2)`, pid, params.ID)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM feedback WHERE project_id = $1 AND (to_id = $2 OR from_id = $2)`, pid, params.ID)
	return err
}
