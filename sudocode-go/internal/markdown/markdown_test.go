package markdown

import (
	"os"
	"path/filepath"
	"testing"

	"encore.app/sudocode-go/internal/models"
)

func strPtr(s string) *string { return &s }

// sampleSpecMarkdown returns markdown matching the TS implementation output format.
func sampleSpecMarkdown() string {
	return `---
id: s-14sh
title: Authentication Flow
priority: 1
created_at: "2026-04-20T10:00:00Z"
parent_id: s-root
tags:
    - auth
    - security
relationships:
    - to_id: s-user
      to_type: spec
      relationship_type: depends-on
---
## Overview

This spec describes the authentication flow.
`
}

func sampleSpecFile() *SpecFile {
	return &SpecFile{
		Spec: models.SpecJSONL{
			Spec: models.Spec{
				ID:        "s-14sh",
				Title:     "Authentication Flow",
				Priority:  1,
				CreatedAt: "2026-04-20T10:00:00Z",
				ParentID:  strPtr("s-root"),
			},
			Tags: []string{"auth", "security"},
			Relationships: []models.RelationshipJSONL{
				{
					From:     "s-14sh",
					FromType: models.EntityTypeSpec,
					To:       "s-user",
					ToType:   models.EntityTypeSpec,
					Type:     models.RelationshipTypeDependsOn,
				},
			},
		},
		Body: "## Overview\n\nThis spec describes the authentication flow.\n",
	}
}

func sampleIssueMarkdown() string {
	return `---
id: i-x7k9
title: Fix Login Bug
priority: 0
created_at: "2026-04-21T08:30:00Z"
parent_id: i-parent
tags:
    - bug
    - urgent
relationships:
    - to_id: s-14sh
      to_type: spec
      relationship_type: implements
status: in_progress
assignee: alice
---
## Steps to Reproduce

1. Go to login page
2. Enter credentials
3. Click submit
`
}

func sampleIssueFile() *IssueFile {
	return &IssueFile{
		Issue: models.IssueJSONL{
			Issue: models.Issue{
				ID:        "i-x7k9",
				Title:     "Fix Login Bug",
				Priority:  0,
				Status:    models.IssueStatusInProgress,
				CreatedAt: "2026-04-21T08:30:00Z",
				ParentID:  strPtr("i-parent"),
				Assignee:  strPtr("alice"),
			},
			Tags: []string{"bug", "urgent"},
			Relationships: []models.RelationshipJSONL{
				{
					From:     "i-x7k9",
					FromType: models.EntityTypeIssue,
					To:       "s-14sh",
					ToType:   models.EntityTypeSpec,
					Type:     models.RelationshipTypeImplements,
				},
			},
		},
		Body: "## Steps to Reproduce\n\n1. Go to login page\n2. Enter credentials\n3. Click submit\n",
	}
}

func TestParseSpec(t *testing.T) {
	sf, err := ParseSpec([]byte(sampleSpecMarkdown()))
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if sf.Spec.ID != "s-14sh" {
		t.Errorf("ID = %q, want %q", sf.Spec.ID, "s-14sh")
	}
	if sf.Spec.Title != "Authentication Flow" {
		t.Errorf("Title = %q, want %q", sf.Spec.Title, "Authentication Flow")
	}
	if sf.Spec.Priority != 1 {
		t.Errorf("Priority = %d, want 1", sf.Spec.Priority)
	}
	if sf.Spec.ParentID == nil || *sf.Spec.ParentID != "s-root" {
		t.Errorf("ParentID = %v, want s-root", sf.Spec.ParentID)
	}
	if len(sf.Spec.Tags) != 2 || sf.Spec.Tags[0] != "auth" {
		t.Errorf("Tags = %v, want [auth security]", sf.Spec.Tags)
	}
	if len(sf.Spec.Relationships) != 1 {
		t.Fatalf("Relationships len = %d, want 1", len(sf.Spec.Relationships))
	}
	r := sf.Spec.Relationships[0]
	if r.To != "s-user" || r.Type != models.RelationshipTypeDependsOn {
		t.Errorf("Relationship = %+v", r)
	}
	if sf.Body != "## Overview\n\nThis spec describes the authentication flow.\n" {
		t.Errorf("Body = %q", sf.Body)
	}
}

func TestParseIssue(t *testing.T) {
	isf, err := ParseIssue([]byte(sampleIssueMarkdown()))
	if err != nil {
		t.Fatalf("ParseIssue: %v", err)
	}
	if isf.Issue.ID != "i-x7k9" {
		t.Errorf("ID = %q", isf.Issue.ID)
	}
	if isf.Issue.Status != models.IssueStatusInProgress {
		t.Errorf("Status = %q", isf.Issue.Status)
	}
	if isf.Issue.Assignee == nil || *isf.Issue.Assignee != "alice" {
		t.Errorf("Assignee = %v", isf.Issue.Assignee)
	}
	if len(isf.Issue.Relationships) != 1 || isf.Issue.Relationships[0].Type != models.RelationshipTypeImplements {
		t.Errorf("Relationships = %+v", isf.Issue.Relationships)
	}
}

func TestRoundTripSpec(t *testing.T) {
	original := sampleSpecFile()

	data, err := MarshalSpec(original)
	if err != nil {
		t.Fatalf("MarshalSpec: %v", err)
	}

	parsed, err := ParseSpec(data)
	if err != nil {
		t.Fatalf("ParseSpec round-trip: %v", err)
	}

	assertSpecEqual(t, &original.Spec, &parsed.Spec)
	if parsed.Body != original.Body {
		t.Errorf("Body mismatch:\n  got:  %q\n  want: %q", parsed.Body, original.Body)
	}
}

func TestRoundTripIssue(t *testing.T) {
	original := sampleIssueFile()

	data, err := MarshalIssue(original)
	if err != nil {
		t.Fatalf("MarshalIssue: %v", err)
	}

	parsed, err := ParseIssue(data)
	if err != nil {
		t.Fatalf("ParseIssue round-trip: %v", err)
	}

	assertIssueEqual(t, &original.Issue, &parsed.Issue)
	if parsed.Body != original.Body {
		t.Errorf("Body mismatch:\n  got:  %q\n  want: %q", parsed.Body, original.Body)
	}
}

func TestRoundTripSpecFile(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	original := sampleSpecFile()
	fname := SpecFilename(original.Spec.ID, original.Spec.Title)
	fpath := filepath.Join(specsDir, fname)

	data, err := MarshalSpec(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fpath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	readBack, err := os.ReadFile(fpath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSpec(readBack)
	if err != nil {
		t.Fatal(err)
	}

	assertSpecEqual(t, &original.Spec, &parsed.Spec)

	if fname != "s-14sh_authentication-flow.md" {
		t.Errorf("filename = %q, want s-14sh_authentication-flow.md", fname)
	}
}

func TestRoundTripIssueFile(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	original := sampleIssueFile()
	fname := IssueFilename(original.Issue.ID, original.Issue.Title)
	fpath := filepath.Join(issuesDir, fname)

	data, err := MarshalIssue(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fpath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	readBack, err := os.ReadFile(fpath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseIssue(readBack)
	if err != nil {
		t.Fatal(err)
	}

	assertIssueEqual(t, &original.Issue, &parsed.Issue)

	if fname != "i-x7k9_fix-login-bug.md" {
		t.Errorf("filename = %q, want i-x7k9_fix-login-bug.md", fname)
	}
}

func TestSpecMinimalFields(t *testing.T) {
	md := "---\nid: s-min\ntitle: Minimal\npriority: 2\ncreated_at: \"2026-01-01T00:00:00Z\"\n---\nHello\n"
	sf, err := ParseSpec([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if sf.Spec.ID != "s-min" {
		t.Errorf("ID = %q", sf.Spec.ID)
	}
	if sf.Spec.ParentID != nil {
		t.Errorf("ParentID should be nil, got %v", sf.Spec.ParentID)
	}
	if len(sf.Spec.Tags) != 0 {
		t.Errorf("Tags should be empty, got %v", sf.Spec.Tags)
	}
	if len(sf.Spec.Relationships) != 0 {
		t.Errorf("Relationships should be empty, got %v", sf.Spec.Relationships)
	}
}

func TestIssueWithClosedAt(t *testing.T) {
	md := `---
id: i-done
title: Done Issue
priority: 2
created_at: "2026-01-01T00:00:00Z"
status: closed
closed_at: "2026-01-02T00:00:00Z"
---
Completed.
`
	isf, err := ParseIssue([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if isf.Issue.Status != models.IssueStatusClosed {
		t.Errorf("Status = %q", isf.Issue.Status)
	}
	if isf.Issue.ClosedAt == nil || *isf.Issue.ClosedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("ClosedAt = %v", isf.Issue.ClosedAt)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"Fix Login Bug!", "fix-login-bug"},
		{"  Multiple   Spaces  ", "multiple-spaces"},
		{"already-slugified", "already-slugified"},
		{"UPPERCASE", "uppercase"},
		{"special@#$chars", "specialchars"},
	}
	for _, tt := range tests {
		got := Slugify(tt.in)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// helpers

func assertSpecEqual(t *testing.T, want, got *models.SpecJSONL) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Priority != want.Priority {
		t.Errorf("Priority = %d, want %d", got.Priority, want.Priority)
	}
	if got.CreatedAt != want.CreatedAt {
		t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, want.CreatedAt)
	}
	assertStringPtrEqual(t, "ParentID", want.ParentID, got.ParentID)
	assertStringSliceEqual(t, "Tags", want.Tags, got.Tags)
	if len(got.Relationships) != len(want.Relationships) {
		t.Fatalf("Relationships len = %d, want %d", len(got.Relationships), len(want.Relationships))
	}
	for i := range want.Relationships {
		if got.Relationships[i] != want.Relationships[i] {
			t.Errorf("Relationships[%d] = %+v, want %+v", i, got.Relationships[i], want.Relationships[i])
		}
	}
}

func assertIssueEqual(t *testing.T, want, got *models.IssueJSONL) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Priority != want.Priority {
		t.Errorf("Priority = %d, want %d", got.Priority, want.Priority)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q, want %q", got.Status, want.Status)
	}
	if got.CreatedAt != want.CreatedAt {
		t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, want.CreatedAt)
	}
	assertStringPtrEqual(t, "ParentID", want.ParentID, got.ParentID)
	assertStringPtrEqual(t, "Assignee", want.Assignee, got.Assignee)
	assertStringPtrEqual(t, "ClosedAt", want.ClosedAt, got.ClosedAt)
	assertStringSliceEqual(t, "Tags", want.Tags, got.Tags)
	if len(got.Relationships) != len(want.Relationships) {
		t.Fatalf("Relationships len = %d, want %d", len(got.Relationships), len(want.Relationships))
	}
	for i := range want.Relationships {
		if got.Relationships[i] != want.Relationships[i] {
			t.Errorf("Relationships[%d] = %+v, want %+v", i, got.Relationships[i], want.Relationships[i])
		}
	}
}

func assertStringPtrEqual(t *testing.T, name string, want, got *string) {
	t.Helper()
	if want == nil && got == nil {
		return
	}
	if want == nil || got == nil {
		t.Errorf("%s: got %v, want %v", name, got, want)
		return
	}
	if *want != *got {
		t.Errorf("%s = %q, want %q", name, *got, *want)
	}
}

func assertStringSliceEqual(t *testing.T, name string, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("%s len = %d, want %d (%v vs %v)", name, len(got), len(want), got, want)
		return
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("%s[%d] = %q, want %q", name, i, got[i], want[i])
		}
	}
}
