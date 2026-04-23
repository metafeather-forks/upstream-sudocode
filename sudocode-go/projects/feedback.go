package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"encore.dev/beta/errs"
)

// --- Request/Response types ---

type CreateFeedbackParams struct {
	ID           *string `json:"id,omitempty"`
	FromID       *string `json:"from_id,omitempty"`
	ToID         string  `json:"to_id"`
	FeedbackType string  `json:"feedback_type"`
	Content      string  `json:"content"`
	Agent        *string `json:"agent,omitempty"`
	Anchor       *string `json:"anchor,omitempty"` // JSON string
}

type UpdateFeedbackParams struct {
	ID        string  `json:"id"`
	Content   *string `json:"content,omitempty"`
	Dismissed *bool   `json:"dismissed,omitempty"`
	Anchor    *string `json:"anchor,omitempty"`
}

type ListFeedbackParams struct {
	ToID string `json:"to_id,omitempty"`
}

type FeedbackResponse struct {
	ID           string  `json:"id"`
	FromID       *string `json:"from_id,omitempty"`
	ToID         string  `json:"to_id"`
	FeedbackType string  `json:"feedback_type"`
	Content      string  `json:"content"`
	Agent        *string `json:"agent,omitempty"`
	Anchor       *string `json:"anchor,omitempty"`
	Dismissed    bool    `json:"dismissed"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type ListFeedbackResponse struct {
	Feedback []FeedbackResponse `json:"feedback"`
}

// --- Helpers ---

func generateID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "f-" + hex.EncodeToString(b)
}

// --- Endpoints ---

// CreateFeedback creates feedback on an entity.
//
//encore:api auth method=POST path=/feedback/create
func CreateFeedback(ctx context.Context, params *CreateFeedbackParams) (*FeedbackResponse, error) {
	pid := projectID(ctx)

	if params.ToID == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "to_id is required"}
	}

	id := generateID()
	if params.ID != nil && *params.ID != "" {
		id = *params.ID
	}

	var createdAt, updatedAt time.Time
	err := db.QueryRow(ctx, `
		INSERT INTO feedback (project_id, id, from_id, to_id, feedback_type, content, agent, anchor)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
		RETURNING created_at, updated_at
	`, pid, id, params.FromID, params.ToID, params.FeedbackType, params.Content, params.Agent, params.Anchor).Scan(&createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	return &FeedbackResponse{
		ID:           id,
		FromID:       params.FromID,
		ToID:         params.ToID,
		FeedbackType: params.FeedbackType,
		Content:      params.Content,
		Agent:        params.Agent,
		Anchor:       params.Anchor,
		Dismissed:    false,
		CreatedAt:    createdAt.Format(time.RFC3339),
		UpdatedAt:    updatedAt.Format(time.RFC3339),
	}, nil
}

// UpdateFeedback updates feedback.
//
//encore:api auth method=POST path=/feedback/update
func UpdateFeedback(ctx context.Context, params *UpdateFeedbackParams) (*FeedbackResponse, error) {
	pid := projectID(ctx)

	if params.Content != nil {
		_, err := db.Exec(ctx, `UPDATE feedback SET content = $1, updated_at = NOW() WHERE project_id = $2 AND id = $3`, *params.Content, pid, params.ID)
		if err != nil {
			return nil, err
		}
	}
	if params.Dismissed != nil {
		_, err := db.Exec(ctx, `UPDATE feedback SET dismissed = $1, updated_at = NOW() WHERE project_id = $2 AND id = $3`, *params.Dismissed, pid, params.ID)
		if err != nil {
			return nil, err
		}
	}
	if params.Anchor != nil {
		_, err := db.Exec(ctx, `UPDATE feedback SET anchor = $1::jsonb, updated_at = NOW() WHERE project_id = $2 AND id = $3`, *params.Anchor, pid, params.ID)
		if err != nil {
			return nil, err
		}
	}

	// Re-read
	var f FeedbackResponse
	var dismissed bool
	var createdAt, updatedAt time.Time
	err := db.QueryRow(ctx, `
		SELECT id, from_id, to_id, feedback_type, content, agent, anchor::text, dismissed, created_at, updated_at
		FROM feedback WHERE project_id = $1 AND id = $2
	`, pid, params.ID).Scan(&f.ID, &f.FromID, &f.ToID, &f.FeedbackType, &f.Content, &f.Agent, &f.Anchor, &dismissed, &createdAt, &updatedAt)
	if err != nil {
		return nil, &errs.Error{Code: errs.NotFound, Message: "feedback not found"}
	}
	f.Dismissed = dismissed
	f.CreatedAt = createdAt.Format(time.RFC3339)
	f.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &f, nil
}

// ListFeedback returns feedback for the authenticated project.
//
//encore:api auth method=POST path=/feedback/list
func ListFeedback(ctx context.Context, params *ListFeedbackParams) (*ListFeedbackResponse, error) {
	pid := projectID(ctx)

	query := `SELECT id, from_id, to_id, feedback_type, content, agent, anchor::text, dismissed, created_at, updated_at FROM feedback WHERE project_id = $1`
	args := []interface{}{pid}

	if params.ToID != "" {
		query += ` AND to_id = $2`
		args = append(args, params.ToID)
	}

	query += ` ORDER BY created_at DESC`

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feedback []FeedbackResponse
	for rows.Next() {
		var f FeedbackResponse
		var dismissed bool
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&f.ID, &f.FromID, &f.ToID, &f.FeedbackType, &f.Content, &f.Agent, &f.Anchor, &dismissed, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		f.Dismissed = dismissed
		f.CreatedAt = createdAt.Format(time.RFC3339)
		f.UpdatedAt = updatedAt.Format(time.RFC3339)
		feedback = append(feedback, f)
	}
	if feedback == nil {
		feedback = []FeedbackResponse{}
	}
	return &ListFeedbackResponse{Feedback: feedback}, rows.Err()
}
