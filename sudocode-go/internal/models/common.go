package models

// EntityType represents the type of a sudocode entity.
type EntityType string

const (
	EntityTypeSpec  EntityType = "spec"
	EntityTypeIssue EntityType = "issue"
)

// RelationshipType represents the kind of relationship between entities.
type RelationshipType string

const (
	RelationshipTypeBlocks        RelationshipType = "blocks"
	RelationshipTypeRelated       RelationshipType = "related"
	RelationshipTypeDiscoveredFrom RelationshipType = "discovered-from"
	RelationshipTypeImplements    RelationshipType = "implements"
	RelationshipTypeReferences    RelationshipType = "references"
	RelationshipTypeDependsOn     RelationshipType = "depends-on"
)

// SyncDirection represents the direction of sync for an external link.
type SyncDirection string

const (
	SyncDirectionInbound       SyncDirection = "inbound"
	SyncDirectionOutbound      SyncDirection = "outbound"
	SyncDirectionBidirectional SyncDirection = "bidirectional"
)

// ExternalLink represents a link to an external system.
type ExternalLink struct {
	Provider          string                 `json:"provider"`
	ExternalID        string                 `json:"external_id"`
	ExternalURL       *string                `json:"external_url,omitempty"`
	SyncEnabled       bool                   `json:"sync_enabled"`
	SyncDirection     SyncDirection           `json:"sync_direction"`
	LastSyncedAt      *string                `json:"last_synced_at,omitempty"`
	ExternalUpdatedAt *string                `json:"external_updated_at,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	ImportedAt        *string                `json:"imported_at,omitempty"`
	ContentHash       *string                `json:"content_hash,omitempty"`
	ImportMetadata    map[string]interface{} `json:"import_metadata,omitempty"`
}
