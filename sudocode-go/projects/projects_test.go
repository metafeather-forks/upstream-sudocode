package projects

import (
	"context"
	"testing"

	"encore.dev/beta/auth"

	localauth "encore.app/sudocode-go/auth"
)

func testCtx(projectID string) context.Context {
	return auth.WithContext(context.Background(), auth.UID(projectID), &localauth.Data{ProjectID: projectID})
}

func TestSpecCRUD(t *testing.T) {
	ctx := testCtx("test-project-1")

	// Create
	title := "Test Spec"
	content := "Spec content"
	priority := 1
	tags := []string{"go", "test"}
	spec, err := UpsertSpec(ctx, &UpsertSpecParams{
		ID:       "s-test1",
		Title:    &title,
		Content:  &content,
		Priority: &priority,
		Tags:     tags,
	})
	if err != nil {
		t.Fatalf("UpsertSpec create: %v", err)
	}
	if spec.ID != "s-test1" || spec.Title != title {
		t.Fatalf("unexpected spec: %+v", spec)
	}

	// Show
	shown, err := ShowSpec(ctx, &ShowSpecParams{ID: "s-test1"})
	if err != nil {
		t.Fatalf("ShowSpec: %v", err)
	}
	if shown.Title != title || len(shown.Tags) != 2 {
		t.Fatalf("unexpected shown spec: %+v", shown)
	}

	// Update
	newTitle := "Updated Spec"
	_, err = UpsertSpec(ctx, &UpsertSpecParams{
		ID:       "s-test1",
		Title:    &newTitle,
		Priority: &priority,
	})
	if err != nil {
		t.Fatalf("UpsertSpec update: %v", err)
	}
	shown, err = ShowSpec(ctx, &ShowSpecParams{ID: "s-test1"})
	if err != nil {
		t.Fatalf("ShowSpec after update: %v", err)
	}
	if shown.Title != newTitle {
		t.Fatalf("title not updated: got %q", shown.Title)
	}

	// List
	list, err := ListSpecs(ctx, &ListSpecsParams{})
	if err != nil {
		t.Fatalf("ListSpecs: %v", err)
	}
	if len(list.Specs) == 0 {
		t.Fatal("expected at least 1 spec")
	}

	// Multi-tenant isolation
	ctx2 := testCtx("test-project-2")
	list2, err := ListSpecs(ctx2, &ListSpecsParams{})
	if err != nil {
		t.Fatalf("ListSpecs project 2: %v", err)
	}
	if len(list2.Specs) != 0 {
		t.Fatalf("expected 0 specs for project 2, got %d", len(list2.Specs))
	}

	// Delete
	err = DeleteSpec(ctx, &DeleteSpecParams{ID: "s-test1"})
	if err != nil {
		t.Fatalf("DeleteSpec: %v", err)
	}
	list, err = ListSpecs(ctx, &ListSpecsParams{})
	if err != nil {
		t.Fatalf("ListSpecs after delete: %v", err)
	}
	if len(list.Specs) != 0 {
		t.Fatalf("expected 0 specs after delete, got %d", len(list.Specs))
	}
}

func TestIssueCRUD(t *testing.T) {
	ctx := testCtx("test-project-3")

	title := "Test Issue"
	content := "Issue content"
	priority := 0
	tags := []string{"bug"}
	issue, err := UpsertIssue(ctx, &UpsertIssueParams{
		ID:       "i-test1",
		Title:    &title,
		Content:  &content,
		Priority: &priority,
		Tags:     tags,
	})
	if err != nil {
		t.Fatalf("UpsertIssue create: %v", err)
	}
	if issue.Status != "open" {
		t.Fatalf("expected open status, got %q", issue.Status)
	}

	// Show
	shown, err := ShowIssue(ctx, &ShowIssueParams{ID: "i-test1"})
	if err != nil {
		t.Fatalf("ShowIssue: %v", err)
	}
	if shown.Title != title {
		t.Fatalf("unexpected: %+v", shown)
	}

	// Close
	closedStatus := "closed"
	_, err = UpsertIssue(ctx, &UpsertIssueParams{
		ID:       "i-test1",
		Title:    &title,
		Status:   &closedStatus,
		Priority: &priority,
	})
	if err != nil {
		t.Fatalf("UpsertIssue close: %v", err)
	}
	shown, err = ShowIssue(ctx, &ShowIssueParams{ID: "i-test1"})
	if err != nil {
		t.Fatalf("ShowIssue after close: %v", err)
	}
	if shown.ClosedAt == nil {
		t.Fatal("expected closed_at to be set")
	}

	// Delete
	err = DeleteIssue(ctx, &DeleteIssueParams{ID: "i-test1"})
	if err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
}

func TestRelationshipCRUD(t *testing.T) {
	ctx := testCtx("test-project-4")

	// Create entities first
	title := "Spec"
	priority := 2
	_, err := UpsertSpec(ctx, &UpsertSpecParams{ID: "s-r1", Title: &title, Priority: &priority})
	if err != nil {
		t.Fatalf("setup spec: %v", err)
	}
	iTitle := "Issue"
	_, err = UpsertIssue(ctx, &UpsertIssueParams{ID: "i-r1", Title: &iTitle, Priority: &priority})
	if err != nil {
		t.Fatalf("setup issue: %v", err)
	}

	// Create relationship
	rel, err := CreateRelationship(ctx, &CreateRelationshipParams{
		FromID: "i-r1", FromType: "issue",
		ToID: "s-r1", ToType: "spec",
		RelationshipType: "implements",
	})
	if err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}
	if rel.RelationshipType != "implements" {
		t.Fatalf("unexpected type: %q", rel.RelationshipType)
	}

	// List
	list, err := ListRelationships(ctx, &ListRelationshipsParams{EntityID: "i-r1"})
	if err != nil {
		t.Fatalf("ListRelationships: %v", err)
	}
	if len(list.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(list.Relationships))
	}

	// Delete
	err = DeleteRelationship(ctx, &DeleteRelationshipParams{
		FromID: "i-r1", ToID: "s-r1", RelationshipType: "implements",
	})
	if err != nil {
		t.Fatalf("DeleteRelationship: %v", err)
	}
	list, err = ListRelationships(ctx, &ListRelationshipsParams{EntityID: "i-r1"})
	if err != nil {
		t.Fatalf("ListRelationships after delete: %v", err)
	}
	if len(list.Relationships) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list.Relationships))
	}
}

func TestFeedbackCRUD(t *testing.T) {
	ctx := testCtx("test-project-5")

	// Create target spec
	title := "Spec"
	priority := 2
	_, err := UpsertSpec(ctx, &UpsertSpecParams{ID: "s-fb1", Title: &title, Priority: &priority})
	if err != nil {
		t.Fatalf("setup spec: %v", err)
	}

	// Create feedback
	fb, err := CreateFeedback(ctx, &CreateFeedbackParams{
		ToID:         "s-fb1",
		FeedbackType: "comment",
		Content:      "Looks good",
	})
	if err != nil {
		t.Fatalf("CreateFeedback: %v", err)
	}
	if fb.Content != "Looks good" {
		t.Fatalf("unexpected content: %q", fb.Content)
	}

	// Update
	newContent := "Updated comment"
	dismissed := true
	updated, err := UpdateFeedback(ctx, &UpdateFeedbackParams{
		ID:        fb.ID,
		Content:   &newContent,
		Dismissed: &dismissed,
	})
	if err != nil {
		t.Fatalf("UpdateFeedback: %v", err)
	}
	if updated.Content != newContent || !updated.Dismissed {
		t.Fatalf("update failed: %+v", updated)
	}

	// List
	list, err := ListFeedback(ctx, &ListFeedbackParams{ToID: "s-fb1"})
	if err != nil {
		t.Fatalf("ListFeedback: %v", err)
	}
	if len(list.Feedback) != 1 {
		t.Fatalf("expected 1 feedback, got %d", len(list.Feedback))
	}
}

func TestReadyEndpoint(t *testing.T) {
	ctx := testCtx("test-project-6")

	title := "Ready Issue"
	priority := 1
	_, err := UpsertIssue(ctx, &UpsertIssueParams{ID: "i-ready1", Title: &title, Priority: &priority})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	blockerTitle := "Blocker"
	_, err = UpsertIssue(ctx, &UpsertIssueParams{ID: "i-blocker1", Title: &blockerTitle, Priority: &priority})
	if err != nil {
		t.Fatalf("setup blocker: %v", err)
	}

	blockedTitle := "Blocked Issue"
	_, err = UpsertIssue(ctx, &UpsertIssueParams{ID: "i-blocked1", Title: &blockedTitle, Priority: &priority})
	if err != nil {
		t.Fatalf("setup blocked: %v", err)
	}

	// Create blocking relationship: i-blocker1 blocks i-blocked1
	_, err = CreateRelationship(ctx, &CreateRelationshipParams{
		FromID: "i-blocker1", FromType: "issue",
		ToID: "i-blocked1", ToType: "issue",
		RelationshipType: "blocks",
	})
	if err != nil {
		t.Fatalf("create blocking rel: %v", err)
	}

	// Ready should return i-ready1 and i-blocker1, but not i-blocked1
	ready, err := Ready(ctx)
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}

	readyIDs := map[string]bool{}
	for _, i := range ready.ReadyIssues {
		readyIDs[i.ID] = true
	}

	if !readyIDs["i-ready1"] {
		t.Fatal("i-ready1 should be ready")
	}
	if !readyIDs["i-blocker1"] {
		t.Fatal("i-blocker1 should be ready (it's not blocked by anything)")
	}
	if readyIDs["i-blocked1"] {
		t.Fatal("i-blocked1 should NOT be ready (blocked by i-blocker1)")
	}

	// Close the blocker -> i-blocked1 becomes ready
	closedStatus := "closed"
	_, err = UpsertIssue(ctx, &UpsertIssueParams{
		ID: "i-blocker1", Title: &blockerTitle, Status: &closedStatus, Priority: &priority,
	})
	if err != nil {
		t.Fatalf("close blocker: %v", err)
	}

	ready, err = Ready(ctx)
	if err != nil {
		t.Fatalf("Ready after close: %v", err)
	}
	readyIDs = map[string]bool{}
	for _, i := range ready.ReadyIssues {
		readyIDs[i.ID] = true
	}
	if !readyIDs["i-blocked1"] {
		t.Fatal("i-blocked1 should now be ready after blocker closed")
	}
}
