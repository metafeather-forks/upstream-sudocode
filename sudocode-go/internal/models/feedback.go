package models

// FeedbackType represents the kind of feedback.
type FeedbackType string

const (
	FeedbackTypeComment    FeedbackType = "comment"
	FeedbackTypeSuggestion FeedbackType = "suggestion"
	FeedbackTypeRequest    FeedbackType = "request"
)

// LocationAnchor tracks a position in a markdown document.
type LocationAnchor struct {
	SectionHeading *string `json:"section_heading,omitempty"`
	SectionLevel   *int    `json:"section_level,omitempty"`
	LineNumber     *int    `json:"line_number,omitempty"`
	LineOffset     *int    `json:"line_offset,omitempty"`
	TextSnippet    *string `json:"text_snippet,omitempty"`
	ContextBefore  *string `json:"context_before,omitempty"`
	ContextAfter   *string `json:"context_after,omitempty"`
	ContentHash    *string `json:"content_hash,omitempty"`
}

// OriginalLocation stores the original position before relocation.
type OriginalLocation struct {
	LineNumber     int     `json:"line_number"`
	SectionHeading *string `json:"section_heading,omitempty"`
}

// FeedbackAnchor extends LocationAnchor with change-tracking fields.
type FeedbackAnchor struct {
	LocationAnchor
	AnchorStatus     string            `json:"anchor_status"`
	LastVerifiedAt   *string           `json:"last_verified_at,omitempty"`
	OriginalLocation *OriginalLocation `json:"original_location,omitempty"`
}

// IssueFeedback represents feedback on a spec or issue (DB form).
type IssueFeedback struct {
	ID           string       `json:"id"`
	FromID       *string      `json:"from_id,omitempty"`
	FromUUID     *string      `json:"from_uuid,omitempty"`
	ToID         string       `json:"to_id"`
	ToUUID       string       `json:"to_uuid"`
	FeedbackType FeedbackType `json:"feedback_type"`
	Content      string       `json:"content"`
	Agent        *string      `json:"agent,omitempty"`
	Anchor       *string      `json:"anchor,omitempty"`
	Dismissed    *bool        `json:"dismissed,omitempty"`
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
}

// FeedbackJSONL is the JSONL serialization format for feedback.
type FeedbackJSONL struct {
	ID           string          `json:"id"`
	FromID       *string         `json:"from_id,omitempty"`
	ToID         string          `json:"to_id"`
	FeedbackType FeedbackType    `json:"feedback_type"`
	Content      string          `json:"content"`
	Agent        *string         `json:"agent,omitempty"`
	Anchor       *FeedbackAnchor `json:"anchor,omitempty"`
	Dismissed    *bool           `json:"dismissed,omitempty"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}
