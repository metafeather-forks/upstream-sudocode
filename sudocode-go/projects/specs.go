package projects

import (
	"context"
	"strconv"
	"time"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"

	localauth "encore.app/sudocode-go/auth"
)

// --- Request/Response types ---

type ListSpecsParams struct {
	Search   string `json:"search,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

type ListSpecsResponse struct {
	Specs []SpecResponse `json:"specs"`
}

type SpecResponse struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Priority  int      `json:"priority"`
	Archived  bool     `json:"archived"`
	ParentID  *string  `json:"parent_id,omitempty"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type ShowSpecParams struct {
	ID string `json:"id"`
}

type UpsertSpecParams struct {
	ID       string   `json:"id"`
	Title    *string  `json:"title,omitempty"`
	Content  *string  `json:"content,omitempty"`
	Priority *int     `json:"priority,omitempty"`
	ParentID *string  `json:"parent_id,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Archived *bool    `json:"archived,omitempty"`
}

type DeleteSpecParams struct {
	ID string `json:"id"`
}

// --- Helpers ---

func projectID(ctx context.Context) string {
	data := auth.Data()
	if data == nil {
		return ""
	}
	if d, ok := data.(*localauth.Data); ok {
		return d.ProjectID
	}
	return ""
}

func tagsForEntity(ctx context.Context, pid, entityType, entityID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT tag FROM tags WHERE project_id = $1 AND entity_type = $2 AND entity_id = $3 ORDER BY tag
	`, pid, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, rows.Err()
}

func syncTags(ctx context.Context, pid, entityType, entityID string, tags []string) error {
	_, err := db.Exec(ctx, `DELETE FROM tags WHERE project_id = $1 AND entity_type = $2 AND entity_id = $3`, pid, entityType, entityID)
	if err != nil {
		return err
	}
	for _, t := range tags {
		_, err := db.Exec(ctx, `INSERT INTO tags (project_id, entity_type, entity_id, tag) VALUES ($1, $2, $3, $4)`, pid, entityType, entityID, t)
		if err != nil {
			return err
		}
	}
	return nil
}

// --- Endpoints ---

// ListSpecs returns specs for the authenticated project.
//
//encore:api auth method=POST path=/specs/list
func ListSpecs(ctx context.Context, params *ListSpecsParams) (*ListSpecsResponse, error) {
	pid := projectID(ctx)

	query := `SELECT id, title, content, priority, archived, parent_id, created_at, updated_at FROM specs WHERE project_id = $1`
	args := []interface{}{pid}
	argIdx := 2

	if !params.Archived {
		query += ` AND archived = false`
	}

	if params.Search != "" {
		query += ` AND (title ILIKE '%' || $` + itoa(argIdx) + ` || '%' OR content ILIKE '%' || $` + itoa(argIdx) + ` || '%')`
		args = append(args, params.Search)
		argIdx++
	}

	query += ` ORDER BY created_at DESC`

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

	var specs []SpecResponse
	for rows.Next() {
		var s SpecResponse
		var archived bool
		var parentID *string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&s.ID, &s.Title, &s.Content, &s.Priority, &archived, &parentID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		s.Archived = archived
		s.ParentID = parentID
		s.CreatedAt = createdAt.Format(time.RFC3339)
		s.UpdatedAt = updatedAt.Format(time.RFC3339)
		s.Tags, err = tagsForEntity(ctx, pid, "spec", s.ID)
		if err != nil {
			return nil, err
		}
		specs = append(specs, s)
	}
	if specs == nil {
		specs = []SpecResponse{}
	}
	return &ListSpecsResponse{Specs: specs}, rows.Err()
}

// ShowSpec returns a single spec by ID.
//
//encore:api auth method=POST path=/specs/show
func ShowSpec(ctx context.Context, params *ShowSpecParams) (*SpecResponse, error) {
	pid := projectID(ctx)
	var s SpecResponse
	var archived bool
	var parentID *string
	var createdAt, updatedAt time.Time

	err := db.QueryRow(ctx, `
		SELECT id, title, content, priority, archived, parent_id, created_at, updated_at
		FROM specs WHERE project_id = $1 AND id = $2
	`, pid, params.ID).Scan(&s.ID, &s.Title, &s.Content, &s.Priority, &archived, &parentID, &createdAt, &updatedAt)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, &errs.Error{Code: errs.NotFound, Message: "spec not found"}
		}
		return nil, err
	}
	s.Archived = archived
	s.ParentID = parentID
	s.CreatedAt = createdAt.Format(time.RFC3339)
	s.UpdatedAt = updatedAt.Format(time.RFC3339)
	s.Tags, err = tagsForEntity(ctx, pid, "spec", s.ID)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpsertSpec creates or updates a spec.
//
//encore:api auth method=POST path=/specs/upsert
func UpsertSpec(ctx context.Context, params *UpsertSpecParams) (*SpecResponse, error) {
	pid := projectID(ctx)

	if params.ID == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "id is required"}
	}

	title := ""
	if params.Title != nil {
		title = *params.Title
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
	err := db.QueryRow(ctx, `
		INSERT INTO specs (project_id, id, title, content, priority, parent_id, archived)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project_id, id) DO UPDATE SET
			title = COALESCE(NULLIF($3, ''), specs.title),
			content = CASE WHEN $4 = '' THEN specs.content ELSE $4 END,
			priority = $5,
			parent_id = $6,
			archived = $7,
			archived_at = CASE WHEN $7 AND NOT specs.archived THEN NOW() ELSE specs.archived_at END,
			updated_at = NOW()
		RETURNING created_at, updated_at
	`, pid, params.ID, title, content, priority, params.ParentID, archived).Scan(&createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	if params.Tags != nil {
		if err := syncTags(ctx, pid, "spec", params.ID, params.Tags); err != nil {
			return nil, err
		}
	}

	tags, err := tagsForEntity(ctx, pid, "spec", params.ID)
	if err != nil {
		return nil, err
	}

	return &SpecResponse{
		ID:        params.ID,
		Title:     title,
		Content:   content,
		Priority:  priority,
		Archived:  archived,
		ParentID:  params.ParentID,
		Tags:      tags,
		CreatedAt: createdAt.Format(time.RFC3339),
		UpdatedAt: updatedAt.Format(time.RFC3339),
	}, nil
}

// DeleteSpec deletes a spec by ID.
//
//encore:api auth method=POST path=/specs/delete
func DeleteSpec(ctx context.Context, params *DeleteSpecParams) error {
	pid := projectID(ctx)
	_, err := db.Exec(ctx, `DELETE FROM specs WHERE project_id = $1 AND id = $2`, pid, params.ID)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM tags WHERE project_id = $1 AND entity_type = 'spec' AND entity_id = $2`, pid, params.ID)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM relationships WHERE project_id = $1 AND (from_id = $2 OR to_id = $2)`, pid, params.ID)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM feedback WHERE project_id = $1 AND to_id = $2`, pid, params.ID)
	return err
}

// itoa is a simple int-to-string for query building.
func itoa(i int) string {
	return strconv.Itoa(i)
}
