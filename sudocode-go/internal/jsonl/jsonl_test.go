package jsonl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"encore.app/sudocode-go/internal/models"
)

func TestReadWriteSpecsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".sudocode", "specs.jsonl")

	specs := []models.SpecJSONL{
		{
			Spec: models.Spec{
				ID:        "s-abc1",
				UUID:      "uuid-1",
				Title:     "First Spec",
				FilePath:  "specs/s-abc1.md",
				Content:   "# First\nSome content",
				Priority:  1,
				CreatedAt: "2025-10-24 04:37:05",
				UpdatedAt: "2025-10-25T10:00:00.000Z",
				ParentID:  nil,
			},
			Relationships: []models.RelationshipJSONL{
				{From: "s-abc1", FromType: "spec", To: "s-def2", ToType: "spec", Type: "blocks"},
			},
			Tags: []string{"backend", "api"},
		},
		{
			Spec: models.Spec{
				ID:        "s-def2",
				UUID:      "uuid-2",
				Title:     "Second Spec",
				FilePath:  "specs/s-def2.md",
				Content:   "# Second",
				Priority:  0,
				CreatedAt: "2025-10-23 01:00:00",
				UpdatedAt: "2025-10-24T01:00:00.000Z",
			},
			Relationships: []models.RelationshipJSONL{},
			Tags:          []string{},
		},
	}

	if err := WriteSpecs(path, specs); err != nil {
		t.Fatalf("WriteSpecs: %v", err)
	}

	got, err := ReadSpecs(path, nil)
	if err != nil {
		t.Fatalf("ReadSpecs: %v", err)
	}

	// Writer sorts by created_at; s-def2 (2025-10-23) comes before s-abc1 (2025-10-24).
	if len(got) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(got))
	}
	if got[0].ID != "s-def2" {
		t.Errorf("expected first spec s-def2, got %s", got[0].ID)
	}
	if got[1].ID != "s-abc1" {
		t.Errorf("expected second spec s-abc1, got %s", got[1].ID)
	}

	// Round-trip: write again, read again, should be identical.
	path2 := filepath.Join(dir, ".sudocode", "specs2.jsonl")
	if err := WriteSpecs(path2, got); err != nil {
		t.Fatalf("WriteSpecs round-trip: %v", err)
	}
	got2, err := ReadSpecs(path2, nil)
	if err != nil {
		t.Fatalf("ReadSpecs round-trip: %v", err)
	}
	if !reflect.DeepEqual(got, got2) {
		t.Errorf("round-trip mismatch:\n  got1: %+v\n  got2: %+v", got, got2)
	}
}

func TestReadWriteIssuesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".sudocode", "issues.jsonl")

	dismissed := true
	fromID := "i-from1"
	lineNum := 5
	snippet := "some text"

	issues := []models.IssueJSONL{
		{
			Issue: models.Issue{
				ID:        "i-aaa1",
				UUID:      "uuid-i1",
				Title:     "First Issue",
				Status:    models.IssueStatusOpen,
				Content:   "Do the thing",
				Priority:  0,
				CreatedAt: "2025-10-24 09:53:37",
				UpdatedAt: "2025-10-28T19:15:21.036Z",
			},
			Relationships: []models.RelationshipJSONL{
				{From: "i-aaa1", FromType: "issue", To: "s-abc1", ToType: "spec", Type: "implements"},
			},
			Tags: []string{"phase-1"},
			Feedback: []models.FeedbackJSONL{
				{
					ID:           "fb-1",
					FromID:       &fromID,
					ToID:         "i-aaa1",
					FeedbackType: models.FeedbackTypeComment,
					Content:      "Looks good",
					Dismissed:    &dismissed,
					Anchor: &models.FeedbackAnchor{
						LocationAnchor: models.LocationAnchor{
							LineNumber:  &lineNum,
							TextSnippet: &snippet,
						},
						AnchorStatus: "verified",
					},
					CreatedAt: "2025-10-25 00:00:00",
					UpdatedAt: "2025-10-25 00:00:00",
				},
			},
		},
		{
			Issue: models.Issue{
				ID:        "i-bbb2",
				UUID:      "uuid-i2",
				Title:     "Second Issue",
				Status:    models.IssueStatusClosed,
				Content:   "Done",
				Priority:  2,
				CreatedAt: "2025-10-20 00:00:00",
				UpdatedAt: "2025-10-21 00:00:00",
			},
			Relationships: []models.RelationshipJSONL{},
			Tags:          []string{},
		},
	}

	if err := WriteIssues(path, issues); err != nil {
		t.Fatalf("WriteIssues: %v", err)
	}

	got, err := ReadIssues(path, nil)
	if err != nil {
		t.Fatalf("ReadIssues: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(got))
	}
	// i-bbb2 (2025-10-20) before i-aaa1 (2025-10-24)
	if got[0].ID != "i-bbb2" {
		t.Errorf("expected first issue i-bbb2, got %s", got[0].ID)
	}

	// Round-trip
	path2 := filepath.Join(dir, ".sudocode", "issues2.jsonl")
	if err := WriteIssues(path2, got); err != nil {
		t.Fatalf("WriteIssues round-trip: %v", err)
	}
	got2, err := ReadIssues(path2, nil)
	if err != nil {
		t.Fatalf("ReadIssues round-trip: %v", err)
	}
	if !reflect.DeepEqual(got, got2) {
		t.Errorf("round-trip mismatch")
	}
}

func TestReadMalformedInput(t *testing.T) {
	dir := t.TempDir()

	t.Run("specs error on bad JSON", func(t *testing.T) {
		path := filepath.Join(dir, "bad_specs.jsonl")
		os.WriteFile(path, []byte("{bad json\n"), 0o644)
		_, err := ReadSpecs(path, nil)
		if err == nil {
			t.Error("expected error for malformed specs")
		}
	})

	t.Run("specs skip errors", func(t *testing.T) {
		path := filepath.Join(dir, "skip_specs.jsonl")
		good := `{"id":"s-1","uuid":"u1","title":"T","file_path":"f","content":"c","priority":0,"created_at":"2025-01-01 00:00:00","updated_at":"2025-01-01 00:00:00","parent_id":null,"parent_uuid":null,"relationships":[],"tags":[]}`
		os.WriteFile(path, []byte("{bad\n"+good+"\n"), 0o644)
		specs, err := ReadSpecs(path, &ReadOptions{SkipErrors: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(specs) != 1 {
			t.Errorf("expected 1 spec, got %d", len(specs))
		}
	})

	t.Run("issues error on bad JSON", func(t *testing.T) {
		path := filepath.Join(dir, "bad_issues.jsonl")
		os.WriteFile(path, []byte("not json\n"), 0o644)
		_, err := ReadIssues(path, nil)
		if err == nil {
			t.Error("expected error for malformed issues")
		}
	})

	t.Run("issues skip errors", func(t *testing.T) {
		path := filepath.Join(dir, "skip_issues.jsonl")
		good := `{"id":"i-1","uuid":"u1","title":"T","status":"open","content":"c","priority":0,"created_at":"2025-01-01 00:00:00","updated_at":"2025-01-01 00:00:00","parent_id":null,"parent_uuid":null,"relationships":[],"tags":[]}`
		os.WriteFile(path, []byte("bad\n\n"+good+"\n"), 0o644)
		issues, err := ReadIssues(path, &ReadOptions{SkipErrors: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(issues) != 1 {
			t.Errorf("expected 1 issue, got %d", len(issues))
		}
	})
}

func TestEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0o644)

	specs, err := ReadSpecs(path, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("expected 0 specs, got %d", len(specs))
	}
}

func TestSkipUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "specs.jsonl")

	specs := []models.SpecJSONL{
		{
			Spec: models.Spec{
				ID: "s-1", UUID: "u1", Title: "T", FilePath: "f", Content: "c",
				CreatedAt: "2025-01-01 00:00:00", UpdatedAt: "2025-01-01 00:00:00",
			},
			Relationships: []models.RelationshipJSONL{},
			Tags:          []string{},
		},
	}

	WriteSpecs(path, specs)
	info1, _ := os.Stat(path)

	// Write again — should skip since content unchanged.
	WriteSpecs(path, specs)
	info2, _ := os.Stat(path)

	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("file should not have been rewritten when content is unchanged")
	}
}

func TestTSCompatibleFormat(t *testing.T) {
	// Verify we can read JSON with fields in any order (TS may emit differently).
	dir := t.TempDir()
	path := filepath.Join(dir, "specs.jsonl")

	// Fields in a different order than Go's struct definition.
	line := `{"tags":["a"],"relationships":[],"title":"T","id":"s-1","uuid":"u","file_path":"f","content":"c","priority":0,"created_at":"2025-01-01 00:00:00","updated_at":"2025-01-01 00:00:00","parent_id":null,"parent_uuid":null}`
	os.WriteFile(path, []byte(line+"\n"), 0o644)

	specs, err := ReadSpecs(path, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 || specs[0].ID != "s-1" {
		t.Errorf("failed to parse TS-ordered fields")
	}
	if len(specs[0].Tags) != 1 || specs[0].Tags[0] != "a" {
		t.Errorf("tags not parsed correctly: %v", specs[0].Tags)
	}
}

func TestReadRealTSSpecs(t *testing.T) {
	// Simulate a line from real TS output with null parent fields and archived integer.
	dir := t.TempDir()
	path := filepath.Join(dir, "specs.jsonl")

	line := `{"id":"SPEC-001","uuid":"37d447c6-5f01-435d-b7e8-99d689e597f8","title":"Agent Execution System","file_path":"specs/SPEC-001.md","content":"# Spec","priority":0,"created_at":"2025-10-24 04:37:05","updated_at":"2026-01-27T03:05:53.744Z","parent_id":null,"parent_uuid":null,"relationships":[],"tags":["agents","architecture"]}`
	os.WriteFile(path, []byte(line+"\n"), 0o644)

	specs, err := ReadSpecs(path, nil)
	if err != nil {
		t.Fatalf("error reading TS-produced spec: %v", err)
	}
	if specs[0].ID != "SPEC-001" {
		t.Errorf("expected SPEC-001, got %s", specs[0].ID)
	}
	if specs[0].ParentID != nil {
		t.Errorf("expected nil ParentID")
	}
}

func TestReadRealTSIssues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")

	archived := 1
	line, _ := json.Marshal(models.IssueJSONL{
		Issue: models.Issue{
			ID: "ISSUE-001", UUID: "6a41fb64-d043-415d-911d-76f2536795f4",
			Title: "Server Setup", Status: "closed", Content: "Set up",
			Priority: 0, Archived: &archived,
			CreatedAt: "2025-10-24 09:53:37", UpdatedAt: "2025-11-03T03:10:12.642Z",
		},
		Relationships: []models.RelationshipJSONL{},
		Tags:          []string{"setup"},
	})
	os.WriteFile(path, append(line, '\n'), 0o644)

	issues, err := ReadIssues(path, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if issues[0].ID != "ISSUE-001" {
		t.Errorf("expected ISSUE-001, got %s", issues[0].ID)
	}
	if issues[0].Archived == nil || *issues[0].Archived != 1 {
		t.Errorf("expected archived=1")
	}
}

func TestFileNotFound(t *testing.T) {
	_, err := ReadSpecs("/nonexistent/path/specs.jsonl", nil)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
	_, err = ReadIssues("/nonexistent/path/issues.jsonl", nil)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
