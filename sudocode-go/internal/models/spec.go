package models

// Spec represents a specification entity (DB/core form).
type Spec struct {
	ID            string          `json:"id"`
	UUID          string          `json:"uuid"`
	Title         string          `json:"title"`
	FilePath      string          `json:"file_path"`
	Content       string          `json:"content"`
	Priority      int             `json:"priority"`
	Archived      *int            `json:"archived,omitempty"`
	ArchivedAt    *string         `json:"archived_at,omitempty"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
	ParentID      *string         `json:"parent_id"`
	ParentUUID    *string         `json:"parent_uuid"`
	ExternalLinks []ExternalLink  `json:"external_links,omitempty"`
}

// SpecJSONL is the JSONL serialization format for specs.
type SpecJSONL struct {
	Spec
	Relationships []RelationshipJSONL `json:"relationships"`
	Tags          []string            `json:"tags"`
}
