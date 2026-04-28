package models

// IssueStatus represents the workflow status of an issue.
type IssueStatus string

const (
	IssueStatusOpen        IssueStatus = "open"
	IssueStatusInProgress  IssueStatus = "in_progress"
	IssueStatusBlocked     IssueStatus = "blocked"
	IssueStatusNeedsReview IssueStatus = "needs_review"
	IssueStatusClosed      IssueStatus = "closed"
	IssueStatusWontFix     IssueStatus = "wont_fix"
	IssueStatusDuplicate   IssueStatus = "duplicate"
)

// Issue represents an issue entity (DB/core form).
type Issue struct {
	ID            string          `json:"id"`
	UUID          string          `json:"uuid"`
	Title         string          `json:"title"`
	Status        IssueStatus     `json:"status"`
	Content       string          `json:"content"`
	Priority      int             `json:"priority"`
	Assignee      *string         `json:"assignee"`
	Archived      *int            `json:"archived,omitempty"`
	ArchivedAt    *string         `json:"archived_at,omitempty"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
	ClosedAt      *string         `json:"closed_at,omitempty"`
	ParentID      *string         `json:"parent_id"`
	ParentUUID    *string         `json:"parent_uuid"`
	ExternalLinks []ExternalLink  `json:"external_links,omitempty"`
}

// IssueJSONL is the JSONL serialization format for issues.
type IssueJSONL struct {
	Issue
	Relationships []RelationshipJSONL `json:"relationships"`
	Tags          []string            `json:"tags"`
	Feedback      []FeedbackJSONL     `json:"feedback,omitempty"`
}
