package models

import (
	"encoding/json"
	"testing"
)

func TestSpecJSONLRoundTrip(t *testing.T) {
	input := `{"id":"s-abc1","uuid":"11111111-1111-1111-1111-111111111111","title":"Test Spec","file_path":"specs/s-abc1.md","content":"# Test","priority":1,"archived":1,"archived_at":"2025-01-01T00:00:00Z","created_at":"2025-01-01 00:00:00","updated_at":"2025-01-02T00:00:00Z","parent_id":"s-parent","parent_uuid":"22222222-2222-2222-2222-222222222222","relationships":[{"from":"s-abc1","from_type":"spec","to":"i-xyz9","to_type":"issue","type":"blocks"}],"tags":["tag1","tag2"]}`

	var spec SpecJSONL
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if spec.ID != "s-abc1" {
		t.Errorf("ID = %q, want %q", spec.ID, "s-abc1")
	}
	if spec.Title != "Test Spec" {
		t.Errorf("Title = %q, want %q", spec.Title, "Test Spec")
	}
	if spec.Priority != 1 {
		t.Errorf("Priority = %d, want 1", spec.Priority)
	}
	if spec.Archived == nil || *spec.Archived != 1 {
		t.Errorf("Archived unexpected")
	}
	if spec.ParentID == nil || *spec.ParentID != "s-parent" {
		t.Errorf("ParentID unexpected")
	}
	if len(spec.Relationships) != 1 {
		t.Fatalf("Relationships len = %d, want 1", len(spec.Relationships))
	}
	rel := spec.Relationships[0]
	if rel.From != "s-abc1" || rel.To != "i-xyz9" || rel.Type != RelationshipTypeBlocks {
		t.Errorf("Relationship = %+v", rel)
	}
	if len(spec.Tags) != 2 || spec.Tags[0] != "tag1" {
		t.Errorf("Tags = %v", spec.Tags)
	}

	out, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var roundTrip SpecJSONL
	if err := json.Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("roundtrip unmarshal: %v", err)
	}
	if roundTrip.ID != spec.ID || roundTrip.Title != spec.Title {
		t.Errorf("round-trip mismatch")
	}
}

func TestIssueJSONLRoundTrip(t *testing.T) {
	input := `{"id":"i-x7k9","uuid":"33333333-3333-3333-3333-333333333333","title":"Fix bug","status":"in_progress","content":"Fix it","priority":2,"assignee":"agent-1","created_at":"2025-01-01 00:00:00","updated_at":"2025-01-02T00:00:00Z","closed_at":"2025-01-03T00:00:00Z","parent_id":null,"parent_uuid":null,"relationships":[],"tags":["bug"],"feedback":[{"id":"f-001","from_id":"i-other","to_id":"s-abc1","feedback_type":"comment","content":"Looks good","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}]}`

	var issue IssueJSONL
	if err := json.Unmarshal([]byte(input), &issue); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if issue.Status != IssueStatusInProgress {
		t.Errorf("Status = %q, want %q", issue.Status, IssueStatusInProgress)
	}
	if issue.Assignee == nil || *issue.Assignee != "agent-1" {
		t.Errorf("Assignee unexpected")
	}
	if issue.ClosedAt == nil || *issue.ClosedAt != "2025-01-03T00:00:00Z" {
		t.Errorf("ClosedAt unexpected")
	}
	if len(issue.Feedback) != 1 {
		t.Fatalf("Feedback len = %d, want 1", len(issue.Feedback))
	}
	fb := issue.Feedback[0]
	if fb.FeedbackType != FeedbackTypeComment || fb.Content != "Looks good" {
		t.Errorf("Feedback = %+v", fb)
	}

	out, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var roundTrip IssueJSONL
	if err := json.Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("roundtrip unmarshal: %v", err)
	}
	if roundTrip.ID != issue.ID || roundTrip.Status != issue.Status {
		t.Errorf("round-trip mismatch")
	}
}

func TestRelationshipJSONLFields(t *testing.T) {
	input := `{"from":"s-abc1","from_type":"spec","to":"i-xyz9","to_type":"issue","type":"implements"}`

	var rel RelationshipJSONL
	if err := json.Unmarshal([]byte(input), &rel); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rel.From != "s-abc1" || rel.FromType != EntityTypeSpec {
		t.Errorf("From = %q/%q", rel.From, rel.FromType)
	}
	if rel.Type != RelationshipTypeImplements {
		t.Errorf("Type = %q", rel.Type)
	}
}

func TestFeedbackJSONLWithAnchor(t *testing.T) {
	input := `{"id":"f-002","to_id":"s-abc1","feedback_type":"suggestion","content":"Change this","anchor":{"anchor_status":"valid","line_number":10,"text_snippet":"some text"},"dismissed":false,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`

	var fb FeedbackJSONL
	if err := json.Unmarshal([]byte(input), &fb); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fb.Anchor == nil {
		t.Fatal("Anchor is nil")
	}
	if fb.Anchor.AnchorStatus != "valid" {
		t.Errorf("AnchorStatus = %q", fb.Anchor.AnchorStatus)
	}
	if fb.Anchor.LineNumber == nil || *fb.Anchor.LineNumber != 10 {
		t.Errorf("LineNumber unexpected")
	}
	if fb.Dismissed == nil || *fb.Dismissed != false {
		t.Errorf("Dismissed unexpected")
	}
}

func TestRelationshipDBForm(t *testing.T) {
	input := `{"from_id":"s-abc1","from_uuid":"11111111-1111-1111-1111-111111111111","from_type":"spec","to_id":"i-xyz9","to_uuid":"22222222-2222-2222-2222-222222222222","to_type":"issue","relationship_type":"blocks","created_at":"2025-01-01T00:00:00Z","metadata":"{\"key\":\"val\"}"}`

	var rel Relationship
	if err := json.Unmarshal([]byte(input), &rel); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rel.FromID != "s-abc1" || rel.RelationshipType != RelationshipTypeBlocks {
		t.Errorf("Relationship = %+v", rel)
	}
	if rel.Metadata == nil {
		t.Error("Metadata is nil")
	}
}

func TestIssueFeedbackDBForm(t *testing.T) {
	input := `{"id":"f-003","from_id":"i-src","from_uuid":"aaaa","to_id":"s-tgt","to_uuid":"bbbb","feedback_type":"request","content":"Please fix","agent":"claude","anchor":"{\"line\":5}","dismissed":true,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`

	var fb IssueFeedback
	if err := json.Unmarshal([]byte(input), &fb); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fb.FeedbackType != FeedbackTypeRequest {
		t.Errorf("FeedbackType = %q", fb.FeedbackType)
	}
	if fb.Agent == nil || *fb.Agent != "claude" {
		t.Errorf("Agent unexpected")
	}
	if fb.Dismissed == nil || *fb.Dismissed != true {
		t.Errorf("Dismissed unexpected")
	}
}

// TestSpecJSONLOmitsNulls verifies that optional nil fields are omitted.
func TestSpecJSONLOmitsNulls(t *testing.T) {
	spec := SpecJSONL{
		Spec: Spec{
			ID:        "s-min",
			UUID:      "00000000-0000-0000-0000-000000000000",
			Title:     "Minimal",
			FilePath:  "specs/s-min.md",
			Content:   "",
			Priority:  0,
			CreatedAt: "2025-01-01 00:00:00",
			UpdatedAt: "2025-01-01 00:00:00",
		},
		Relationships: []RelationshipJSONL{},
		Tags:          []string{},
	}

	out, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Should NOT contain archived, archived_at, external_links
	s := string(out)
	for _, field := range []string{"archived", "archived_at", "external_links"} {
		if contains(s, `"`+field+`"`) {
			t.Errorf("output should omit %q, got: %s", field, s)
		}
	}

	// parent_id and parent_uuid should be present as null (matching TS output)
	if !contains(s, `"parent_id":null`) {
		t.Errorf("output should contain parent_id:null, got: %s", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
