package models

// Relationship represents a full relationship between two entities (DB form).
type Relationship struct {
	FromID           string           `json:"from_id"`
	FromUUID         string           `json:"from_uuid"`
	FromType         EntityType       `json:"from_type"`
	ToID             string           `json:"to_id"`
	ToUUID           string           `json:"to_uuid"`
	ToType           EntityType       `json:"to_type"`
	RelationshipType RelationshipType `json:"relationship_type"`
	CreatedAt        string           `json:"created_at"`
	Metadata         *string          `json:"metadata,omitempty"`
}

// RelationshipJSONL is the compact JSONL format for relationships.
type RelationshipJSONL struct {
	From     string           `json:"from"`
	FromType EntityType       `json:"from_type"`
	To       string           `json:"to"`
	ToType   EntityType       `json:"to_type"`
	Type     RelationshipType `json:"type"`
}
