package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"encore.app/sudocode-go/internal/models"
)

// --- helpers ---

func makeSpecJSONL(id, title, content, createdAt string, tags []string, rels []models.RelationshipJSONL) models.SpecJSONL {
	if tags == nil {
		tags = []string{}
	}
	if rels == nil {
		rels = []models.RelationshipJSONL{}
	}
	return models.SpecJSONL{
		Spec: models.Spec{
			ID:        id,
			Title:     title,
			Content:   content,
			Priority:  2,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
		Tags:          tags,
		Relationships: rels,
	}
}

func makeIssueJSONL(id, title, content, createdAt string, status models.IssueStatus, tags []string) models.IssueJSONL {
	if tags == nil {
		tags = []string{}
	}
	return models.IssueJSONL{
		Issue: models.Issue{
			ID:        id,
			Title:     title,
			Content:   content,
			Priority:  1,
			Status:    status,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
		Tags:          tags,
		Relationships: []models.RelationshipJSONL{},
	}
}

func writeSudocodeJSONL(t *testing.T, dir string, specs []models.SpecJSONL, issues []models.IssueJSONL) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write specs.jsonl
	var specLines []string
	for _, s := range specs {
		b, _ := json.Marshal(s)
		specLines = append(specLines, string(b))
	}
	if err := os.WriteFile(filepath.Join(dir, "specs.jsonl"), []byte(strings.Join(specLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write issues.jsonl
	var issueLines []string
	for _, i := range issues {
		b, _ := json.Marshal(i)
		issueLines = append(issueLines, string(b))
	}
	if err := os.WriteFile(filepath.Join(dir, "issues.jsonl"), []byte(strings.Join(issueLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- Config tests ---

func TestReadConfig_Default(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ReadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourceOfTruth != StorageModeJSONL {
		t.Errorf("expected jsonl, got %s", cfg.SourceOfTruth)
	}
}

func TestReadConfig_Explicit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"sourceOfTruth":"markdown"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourceOfTruth != StorageModeMarkdown {
		t.Errorf("expected markdown, got %s", cfg.SourceOfTruth)
	}
}

func TestWriteConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := ProjectConfig{SourceOfTruth: StorageModeMarkdown}
	if err := WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := ReadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceOfTruth != StorageModeMarkdown {
		t.Errorf("round-trip failed: got %s", got.SourceOfTruth)
	}
}

// --- Gitignore tests ---

func TestGitignoreContent_JSONL(t *testing.T) {
	content := GitignoreContent(StorageModeJSONL)
	if !strings.Contains(content, "specs/") || !strings.Contains(content, "issues/") {
		t.Errorf("jsonl mode should ignore markdown dirs, got:\n%s", content)
	}
}

func TestGitignoreContent_Markdown(t *testing.T) {
	content := GitignoreContent(StorageModeMarkdown)
	if !strings.Contains(content, "specs.jsonl") || !strings.Contains(content, "issues.jsonl") {
		t.Errorf("markdown mode should ignore JSONL files, got:\n%s", content)
	}
}

// --- Import JSONL tests ---

func TestImport_JSONL(t *testing.T) {
	dir := t.TempDir()

	specs := []models.SpecJSONL{
		makeSpecJSONL("s-abc1", "Test Spec", "spec content", "2025-01-01T00:00:00Z", []string{"tag1"}, nil),
	}
	issues := []models.IssueJSONL{
		makeIssueJSONL("i-def2", "Test Issue", "issue content", "2025-01-01T00:00:00Z", models.IssueStatusOpen, []string{"bug"}),
	}

	writeSudocodeJSONL(t, dir, specs, issues)

	// Write config
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"sourceOfTruth":"jsonl"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Import(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(result.Specs))
	}
	if result.Specs[0].ID != "s-abc1" {
		t.Errorf("spec ID = %s, want s-abc1", result.Specs[0].ID)
	}
	if result.Specs[0].Title != "Test Spec" {
		t.Errorf("spec title = %s", result.Specs[0].Title)
	}
	if result.Specs[0].Content != "spec content" {
		t.Errorf("spec content = %q", result.Specs[0].Content)
	}

	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}
	if result.Issues[0].ID != "i-def2" {
		t.Errorf("issue ID = %s", result.Issues[0].ID)
	}
}

func TestImport_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	result, err := Import(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Specs) != 0 {
		t.Errorf("expected 0 specs, got %d", len(result.Specs))
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(result.Issues))
	}
}

func TestImport_WithRelationships(t *testing.T) {
	dir := t.TempDir()

	rels := []models.RelationshipJSONL{
		{From: "s-abc1", FromType: models.EntityTypeSpec, To: "i-def2", ToType: models.EntityTypeIssue, Type: models.RelationshipTypeBlocks},
	}
	specs := []models.SpecJSONL{
		makeSpecJSONL("s-abc1", "Spec With Rels", "content", "2025-01-01T00:00:00Z", nil, rels),
	}
	writeSudocodeJSONL(t, dir, specs, nil)
	// Write empty issues
	if err := os.WriteFile(filepath.Join(dir, "issues.jsonl"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Import(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Specs[0].Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(result.Specs[0].Relationships))
	}
	rel := result.Specs[0].Relationships[0]
	if rel.To != "i-def2" || rel.Type != models.RelationshipTypeBlocks {
		t.Errorf("relationship = %+v", rel)
	}
}

// --- Export tests ---

func TestExport_CreatesAllFiles(t *testing.T) {
	dir := t.TempDir()

	specs := []models.SpecJSONL{
		makeSpecJSONL("s-abc1", "My Spec", "spec body", "2025-01-01T00:00:00Z", nil, nil),
	}
	issues := []models.IssueJSONL{
		makeIssueJSONL("i-def2", "My Issue", "issue body", "2025-01-01T00:00:00Z", models.IssueStatusOpen, nil),
	}
	cfg := ProjectConfig{SourceOfTruth: StorageModeJSONL}

	if err := Export(dir, specs, issues, cfg); err != nil {
		t.Fatal(err)
	}

	// Check config.json exists
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Error("config.json not found")
	}
	// Check .gitignore exists
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
		t.Error(".gitignore not found")
	}
	// Check specs.jsonl
	if _, err := os.Stat(filepath.Join(dir, "specs.jsonl")); err != nil {
		t.Error("specs.jsonl not found")
	}
	// Check issues.jsonl
	if _, err := os.Stat(filepath.Join(dir, "issues.jsonl")); err != nil {
		t.Error("issues.jsonl not found")
	}
	// Check markdown files
	specsMd, _ := os.ReadDir(filepath.Join(dir, "specs"))
	if len(specsMd) != 1 {
		t.Errorf("expected 1 spec markdown file, got %d", len(specsMd))
	}
	issuesMd, _ := os.ReadDir(filepath.Join(dir, "issues"))
	if len(issuesMd) != 1 {
		t.Errorf("expected 1 issue markdown file, got %d", len(issuesMd))
	}
}

func TestExport_JSONLAlwaysWritten(t *testing.T) {
	dir := t.TempDir()

	specs := []models.SpecJSONL{
		makeSpecJSONL("s-test", "Test", "body", "2025-01-01T00:00:00Z", nil, nil),
	}
	cfg := ProjectConfig{SourceOfTruth: StorageModeMarkdown}

	if err := Export(dir, specs, nil, cfg); err != nil {
		t.Fatal(err)
	}

	// JSONL must be written even in markdown mode
	if _, err := os.Stat(filepath.Join(dir, "specs.jsonl")); err != nil {
		t.Error("specs.jsonl should always be written, even in markdown mode")
	}
}

// --- Round-trip test ---

func TestImportExport_RoundTrip_JSONL(t *testing.T) {
	dir := t.TempDir()

	specs := []models.SpecJSONL{
		makeSpecJSONL("s-rt01", "Round Trip Spec", "# Hello\nWorld", "2025-01-01T00:00:00Z", []string{"go", "test"}, nil),
	}
	issues := []models.IssueJSONL{
		makeIssueJSONL("i-rt02", "Round Trip Issue", "Fix the bug", "2025-01-02T00:00:00Z", models.IssueStatusInProgress, []string{"urgent"}),
	}
	cfg := ProjectConfig{SourceOfTruth: StorageModeJSONL}

	if err := Export(dir, specs, issues, cfg); err != nil {
		t.Fatal(err)
	}

	result, err := Import(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Specs) != 1 || result.Specs[0].ID != "s-rt01" {
		t.Errorf("spec round-trip failed: %+v", result.Specs)
	}
	if len(result.Specs[0].Tags) != 2 {
		t.Errorf("spec tags lost: %v", result.Specs[0].Tags)
	}

	if len(result.Issues) != 1 || result.Issues[0].ID != "i-rt02" {
		t.Errorf("issue round-trip failed: %+v", result.Issues)
	}
	if result.Issues[0].Status != models.IssueStatusInProgress {
		t.Errorf("issue status = %s, want in_progress", result.Issues[0].Status)
	}
}

// --- Markdown import test ---

func TestImport_Markdown(t *testing.T) {
	dir := t.TempDir()

	// Write config for markdown mode
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"sourceOfTruth":"markdown"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a spec markdown file
	specsDir := filepath.Join(dir, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	specMd := `---
id: s-md01
title: Markdown Spec
priority: 1
created_at: "2025-03-01T00:00:00Z"
tags:
    - alpha
---
This is the spec body.
`
	if err := os.WriteFile(filepath.Join(specsDir, "s-md01_markdown-spec.md"), []byte(specMd), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write an issue markdown file
	issuesDir := filepath.Join(dir, "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	issueMd := `---
id: i-md02
title: Markdown Issue
priority: 0
created_at: "2025-03-02T00:00:00Z"
status: blocked
---
Fix this now.
`
	if err := os.WriteFile(filepath.Join(issuesDir, "i-md02_markdown-issue.md"), []byte(issueMd), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Import(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(result.Specs))
	}
	if result.Specs[0].ID != "s-md01" {
		t.Errorf("spec ID = %s", result.Specs[0].ID)
	}
	if result.Specs[0].Priority != 1 {
		t.Errorf("spec priority = %d", result.Specs[0].Priority)
	}

	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}
	if result.Issues[0].Status != models.IssueStatusBlocked {
		t.Errorf("issue status = %s", result.Issues[0].Status)
	}
}

// --- Markdown fallback to JSONL ---

func TestImport_Markdown_FallsBackToJSONL(t *testing.T) {
	dir := t.TempDir()

	// Markdown mode but no markdown dirs — should fall back to JSONL
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"sourceOfTruth":"markdown"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	specs := []models.SpecJSONL{
		makeSpecJSONL("s-fb01", "Fallback Spec", "content", "2025-01-01T00:00:00Z", nil, nil),
	}
	writeSudocodeJSONL(t, dir, specs, nil)
	// Write empty issues.jsonl
	if err := os.WriteFile(filepath.Join(dir, "issues.jsonl"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Import(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Specs) != 1 || result.Specs[0].ID != "s-fb01" {
		t.Errorf("fallback to JSONL failed: %+v", result.Specs)
	}
}

// --- Conflict resolution ---

func TestResolveConflict(t *testing.T) {
	cfg := ProjectConfig{SourceOfTruth: StorageModeJSONL}
	if r := ResolveConflict(cfg); r != PreferFile {
		t.Errorf("expected PreferFile, got %d", r)
	}
	cfg.SourceOfTruth = StorageModeMarkdown
	if r := ResolveConflict(cfg); r != PreferFile {
		t.Errorf("expected PreferFile, got %d", r)
	}
}

// --- Multiple entities ---

func TestExport_MultipleEntities(t *testing.T) {
	dir := t.TempDir()

	specs := []models.SpecJSONL{
		makeSpecJSONL("s-m01", "First", "body1", "2025-01-01T00:00:00Z", nil, nil),
		makeSpecJSONL("s-m02", "Second", "body2", "2025-01-02T00:00:00Z", nil, nil),
		makeSpecJSONL("s-m03", "Third", "body3", "2025-01-03T00:00:00Z", nil, nil),
	}
	issues := []models.IssueJSONL{
		makeIssueJSONL("i-m01", "Issue One", "b1", "2025-01-01T00:00:00Z", models.IssueStatusOpen, nil),
		makeIssueJSONL("i-m02", "Issue Two", "b2", "2025-01-02T00:00:00Z", models.IssueStatusClosed, nil),
	}
	cfg := ProjectConfig{SourceOfTruth: StorageModeJSONL}

	if err := Export(dir, specs, issues, cfg); err != nil {
		t.Fatal(err)
	}

	result, err := Import(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Specs) != 3 {
		t.Errorf("expected 3 specs, got %d", len(result.Specs))
	}
	if len(result.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(result.Issues))
	}
}
