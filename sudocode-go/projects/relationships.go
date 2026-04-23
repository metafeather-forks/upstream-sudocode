package projects

import (
	"context"
	"time"
)

// --- Request/Response types ---

type CreateRelationshipParams struct {
	FromID           string `json:"from_id"`
	FromType         string `json:"from_type"`
	ToID             string `json:"to_id"`
	ToType           string `json:"to_type"`
	RelationshipType string `json:"relationship_type"`
}

type DeleteRelationshipParams struct {
	FromID           string `json:"from_id"`
	ToID             string `json:"to_id"`
	RelationshipType string `json:"relationship_type"`
}

type ListRelationshipsParams struct {
	EntityID string `json:"entity_id,omitempty"`
}

type RelationshipResponse struct {
	FromID           string `json:"from_id"`
	FromType         string `json:"from_type"`
	ToID             string `json:"to_id"`
	ToType           string `json:"to_type"`
	RelationshipType string `json:"relationship_type"`
	CreatedAt        string `json:"created_at"`
}

type ListRelationshipsResponse struct {
	Relationships []RelationshipResponse `json:"relationships"`
}

// --- Endpoints ---

// CreateRelationship creates a relationship between two entities.
//
//encore:api auth method=POST path=/relationships/create
func CreateRelationship(ctx context.Context, params *CreateRelationshipParams) (*RelationshipResponse, error) {
	pid := projectID(ctx)

	var createdAt time.Time
	err := db.QueryRow(ctx, `
		INSERT INTO relationships (project_id, from_id, from_type, to_id, to_type, relationship_type)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, from_id, to_id, relationship_type) DO UPDATE SET
			from_type = $3, to_type = $5
		RETURNING created_at
	`, pid, params.FromID, params.FromType, params.ToID, params.ToType, params.RelationshipType).Scan(&createdAt)
	if err != nil {
		return nil, err
	}

	return &RelationshipResponse{
		FromID:           params.FromID,
		FromType:         params.FromType,
		ToID:             params.ToID,
		ToType:           params.ToType,
		RelationshipType: params.RelationshipType,
		CreatedAt:        createdAt.Format(time.RFC3339),
	}, nil
}

// DeleteRelationship removes a relationship.
//
//encore:api auth method=POST path=/relationships/delete
func DeleteRelationship(ctx context.Context, params *DeleteRelationshipParams) error {
	pid := projectID(ctx)
	_, err := db.Exec(ctx, `
		DELETE FROM relationships WHERE project_id = $1 AND from_id = $2 AND to_id = $3 AND relationship_type = $4
	`, pid, params.FromID, params.ToID, params.RelationshipType)
	return err
}

// ListRelationships returns relationships for the authenticated project.
//
//encore:api auth method=POST path=/relationships/list
func ListRelationships(ctx context.Context, params *ListRelationshipsParams) (*ListRelationshipsResponse, error) {
	pid := projectID(ctx)

	query := `SELECT from_id, from_type, to_id, to_type, relationship_type, created_at FROM relationships WHERE project_id = $1`
	args := []interface{}{pid}

	if params.EntityID != "" {
		query += ` AND (from_id = $2 OR to_id = $2)`
		args = append(args, params.EntityID)
	}

	query += ` ORDER BY created_at DESC`

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []RelationshipResponse
	for rows.Next() {
		var r RelationshipResponse
		var createdAt time.Time
		if err := rows.Scan(&r.FromID, &r.FromType, &r.ToID, &r.ToType, &r.RelationshipType, &createdAt); err != nil {
			return nil, err
		}
		r.CreatedAt = createdAt.Format(time.RFC3339)
		rels = append(rels, r)
	}
	if rels == nil {
		rels = []RelationshipResponse{}
	}
	return &ListRelationshipsResponse{Relationships: rels}, rows.Err()
}
