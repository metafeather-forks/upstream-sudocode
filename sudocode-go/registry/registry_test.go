package registry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTestStore(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(dir)
	old := store
	SetStore(s)
	return dir, func() { store = old }
}

func loadJSON(t *testing.T, dir string) ProjectsConfig {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "projects.json"))
	if err != nil {
		t.Fatalf("read projects.json: %v", err)
	}
	var cfg ProjectsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return cfg
}

func TestStoreDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	cfg, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	if len(cfg.Projects) != 0 {
		t.Errorf("projects = %d, want 0", len(cfg.Projects))
	}
	if cfg.Settings.MaxRecentProjects != 10 {
		t.Errorf("maxRecentProjects = %d, want 10", cfg.Settings.MaxRecentProjects)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	cfg := defaultConfig()
	cfg.Projects["test-1234"] = ProjectInfo{
		ID:           "test-1234",
		Name:         "test",
		Path:         "/tmp/test",
		SudocodeDir:  "/tmp/test/.sudocode",
		RegisteredAt: "2025-01-01T00:00:00Z",
		LastOpenedAt: "2025-01-01T00:00:00Z",
	}
	cfg.RecentProjects = []string{"test-1234"}
	cfg.CurrentProjectID = "test-1234"
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentProjectID != "test-1234" {
		t.Errorf("currentProjectId = %q, want %q", loaded.CurrentProjectID, "test-1234")
	}
	p := loaded.Projects["test-1234"]
	if p.Name != "test" || p.Path != "/tmp/test" {
		t.Errorf("project mismatch: %+v", p)
	}
}

func TestStoreCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "projects.json"), []byte("{bad json"), 0o644)
	s := NewStore(dir)
	cfg, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Errorf("expected default config on corruption, got version %d", cfg.Version)
	}
}

func TestOpen(t *testing.T) {
	dir, cleanup := setupTestStore(t)
	defer cleanup()

	projDir := t.TempDir()
	ctx := context.Background()

	resp, err := Open(ctx, &OpenParams{Path: projDir})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Project.Path != projDir {
		t.Errorf("path = %q, want %q", resp.Project.Path, projDir)
	}
	if resp.Project.ID == "" {
		t.Error("id is empty")
	}
	if resp.Project.RegisteredAt == "" {
		t.Error("registeredAt is empty")
	}

	// Verify persisted
	cfg := loadJSON(t, dir)
	if cfg.CurrentProjectID != resp.Project.ID {
		t.Errorf("currentProjectId = %q, want %q", cfg.CurrentProjectID, resp.Project.ID)
	}
	if len(cfg.RecentProjects) != 1 || cfg.RecentProjects[0] != resp.Project.ID {
		t.Errorf("recentProjects = %v", cfg.RecentProjects)
	}
}

func TestOpenIdempotent(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	projDir := t.TempDir()
	ctx := context.Background()

	r1, _ := Open(ctx, &OpenParams{Path: projDir})
	r2, _ := Open(ctx, &OpenParams{Path: projDir})

	if r1.Project.ID != r2.Project.ID {
		t.Error("reopening same path should return same ID")
	}
	if r2.Project.RegisteredAt != r1.Project.RegisteredAt {
		t.Error("registeredAt should not change on reopen")
	}
}

func TestClose(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	projDir := t.TempDir()
	ctx := context.Background()

	resp, _ := Open(ctx, &OpenParams{Path: projDir})
	_, err := Close(ctx, &CloseParams{ProjectID: resp.Project.ID})
	if err != nil {
		t.Fatal(err)
	}

	cur, _ := Current(ctx)
	if cur.ProjectID != "" {
		t.Errorf("currentProjectId should be empty after close, got %q", cur.ProjectID)
	}
}

func TestCloseNotFound(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := Close(context.Background(), &CloseParams{ProjectID: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}

func TestInit(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	projDir := t.TempDir()
	ctx := context.Background()
	name := "my-project"

	resp, err := Init(ctx, &InitParams{Path: projDir, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Project.Name != "my-project" {
		t.Errorf("name = %q, want %q", resp.Project.Name, "my-project")
	}

	// .sudocode dir should exist
	info, err := os.Stat(filepath.Join(projDir, ".sudocode"))
	if err != nil || !info.IsDir() {
		t.Error(".sudocode directory was not created")
	}
}

func TestValidate(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	projDir := t.TempDir()
	ctx := context.Background()

	// Without .sudocode
	resp, err := Validate(ctx, &ValidateParams{Path: projDir})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Valid {
		t.Error("expected valid=true for existing dir")
	}
	if resp.HasSudocode {
		t.Error("expected hasSudocode=false")
	}

	// Create .sudocode
	os.MkdirAll(filepath.Join(projDir, ".sudocode"), 0o755)
	resp, _ = Validate(ctx, &ValidateParams{Path: projDir})
	if !resp.HasSudocode {
		t.Error("expected hasSudocode=true")
	}

	// Nonexistent path
	resp, _ = Validate(ctx, &ValidateParams{Path: "/nonexistent/path/xyz"})
	if resp.Valid {
		t.Error("expected valid=false for nonexistent path")
	}
}

func TestBrowse(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hi"), 0o644)

	resp, err := Browse(context.Background(), &BrowseParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if resp.CurrentPath != dir {
		t.Errorf("currentPath = %q, want %q", resp.CurrentPath, dir)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(resp.Entries))
	}
}

func TestCurrentAndSetCurrent(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	projDir := t.TempDir()

	resp, _ := Open(ctx, &OpenParams{Path: projDir})

	cur, _ := Current(ctx)
	if cur.ProjectID != resp.Project.ID {
		t.Errorf("current = %q, want %q", cur.ProjectID, resp.Project.ID)
	}
	if cur.Project == nil {
		t.Error("expected project info")
	}
}

func TestRecent(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	Open(ctx, &OpenParams{Path: dir1})
	Open(ctx, &OpenParams{Path: dir2})

	resp, err := Recent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Projects) != 2 {
		t.Fatalf("recent = %d, want 2", len(resp.Projects))
	}
	// Most recent first
	if resp.Projects[0].Path != dir2 {
		t.Errorf("most recent should be dir2, got %q", resp.Projects[0].Path)
	}
}

func TestRecentMaxLimit(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	// Open 12 projects, recent should cap at 10
	for i := 0; i < 12; i++ {
		d := t.TempDir()
		Open(ctx, &OpenParams{Path: d})
	}

	resp, _ := Recent(ctx)
	if len(resp.Projects) > 10 {
		t.Errorf("recent = %d, want <= 10", len(resp.Projects))
	}
}

func TestList(t *testing.T) {
	_, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	Open(ctx, &OpenParams{Path: t.TempDir()})
	Open(ctx, &OpenParams{Path: t.TempDir()})

	resp, err := List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Projects) != 2 {
		t.Errorf("list = %d, want 2", len(resp.Projects))
	}
}

func TestJSONFormat(t *testing.T) {
	dir, cleanup := setupTestStore(t)
	defer cleanup()

	projDir := t.TempDir()
	Open(context.Background(), &OpenParams{Path: projDir})

	// Verify the raw JSON has the expected TS-compatible fields
	data, err := os.ReadFile(filepath.Join(dir, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, ok := raw["version"]; !ok {
		t.Error("missing 'version' key")
	}
	if _, ok := raw["projects"]; !ok {
		t.Error("missing 'projects' key")
	}
	if _, ok := raw["recentProjects"]; !ok {
		t.Error("missing 'recentProjects' key")
	}
	if _, ok := raw["settings"]; !ok {
		t.Error("missing 'settings' key")
	}
	if _, ok := raw["currentProjectId"]; !ok {
		t.Error("missing 'currentProjectId' key")
	}

	// Check project fields
	projects := raw["projects"].(map[string]interface{})
	for _, v := range projects {
		p := v.(map[string]interface{})
		for _, key := range []string{"id", "name", "path", "sudocodeDir", "registeredAt", "lastOpenedAt"} {
			if _, ok := p[key]; !ok {
				t.Errorf("missing project key %q", key)
			}
		}
	}
}
