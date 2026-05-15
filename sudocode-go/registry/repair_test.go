package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"encore.app/sudocode-go/internal/sudocodefile"
	syncpkg "encore.app/sudocode-go/sync"
)

// setupRepairProject creates a valid project with forward and back links.
// Returns (configDir, repoPath, sudocodeDir, projectID).
func setupRepairProject(t *testing.T, configDir string, colocated bool) (string, string, string) {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), "myrepo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}

	var sudocodeDir string
	if colocated {
		sudocodeDir = filepath.Join(repoPath, ".sudocode")
	} else {
		sudocodeDir = filepath.Join(t.TempDir(), "external-sudocode")
	}
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	projectID := GenerateProjectID(repoPath)

	// Write config.json with projectId
	if err := syncpkg.WriteConfig(sudocodeDir, syncpkg.ProjectConfig{
		SourceOfTruth: syncpkg.StorageModeJSONL,
		ProjectID:     projectID,
	}); err != nil {
		t.Fatal(err)
	}

	// Write config.local.json with projectdir back-link
	if err := syncpkg.WriteLocalConfig(sudocodeDir, syncpkg.LocalConfig{
		ProjectDir: repoPath,
	}); err != nil {
		t.Fatal(err)
	}

	// For external mode, write .sudocode pointer file in repo
	if !colocated {
		if err := sudocodefile.WriteSudocodeFile(repoPath, sudocodeDir); err != nil {
			t.Fatal(err)
		}
	}

	// Register in projects.json
	s := NewStore(configDir)
	SetStore(s)
	cfg := defaultConfig()
	cfg.Projects[projectID] = ProjectInfo{
		ID:           projectID,
		Name:         "myrepo",
		SudocodeDir:  sudocodeDir,
		RegisteredAt: "2025-01-01T00:00:00Z",
		LastOpenedAt: "2025-01-01T00:00:00Z",
	}
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}

	return repoPath, sudocodeDir, projectID
}

func TestRepair_AllLinksValid(t *testing.T) {
	configDir := t.TempDir()
	s := NewStore(configDir)
	old := store
	SetStore(s)
	defer func() { store = old }()

	setupRepairProject(t, configDir, true)

	resp, err := Repair(context.Background(), &RepairParams{Fix: false, Rebuild: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d: %v", len(resp.Issues), resp.Issues)
	}
}

func TestRepair_BrokenForwardLink(t *testing.T) {
	configDir := t.TempDir()
	s := NewStore(configDir)
	old := store
	SetStore(s)
	defer func() { store = old }()

	repoPath, sudocodeDir, _ := setupRepairProject(t, configDir, false)

	// Break the forward link: .sudocode file points to wrong dir
	wrongDir := filepath.Join(t.TempDir(), "wrong-sudocode")
	if err := os.MkdirAll(wrongDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := sudocodefile.WriteSudocodeFile(repoPath, wrongDir); err != nil {
		t.Fatal(err)
	}

	// Dry run — should report the issue
	resp, err := Repair(context.Background(), &RepairParams{Fix: false})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range resp.Issues {
		if issue.Type == "forward_link_mismatch" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected forward_link_mismatch issue, got: %v", resp.Issues)
	}

	// Fix mode — should repair it
	resp, err = Repair(context.Background(), &RepairParams{Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	foundFix := false
	for _, a := range resp.Actions {
		if a.Type == "fixed_forward_link" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Errorf("expected fixed_forward_link action, got: %v", resp.Actions)
	}

	// Verify fix: .sudocode file should now point to correct dir
	resolved, err := sudocodefile.ResolveSudocodeDir(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != sudocodeDir {
		t.Errorf("after fix: resolved=%q, want %q", resolved, sudocodeDir)
	}
}

func TestRepair_BrokenBackLink(t *testing.T) {
	configDir := t.TempDir()
	s := NewStore(configDir)
	old := store
	SetStore(s)
	defer func() { store = old }()

	_, sudocodeDir, _ := setupRepairProject(t, configDir, true)

	// Break the back-link: projectdir points to wrong repo
	wrongRepo := filepath.Join(t.TempDir(), "wrong-repo")
	if err := os.MkdirAll(wrongRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syncpkg.WriteLocalConfig(sudocodeDir, syncpkg.LocalConfig{
		ProjectDir: wrongRepo,
	}); err != nil {
		t.Fatal(err)
	}

	// Dry run — should report the issue
	resp, err := Repair(context.Background(), &RepairParams{Fix: false})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range resp.Issues {
		if issue.Type == "back_link_target_no_sudocode" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected back_link_target_no_sudocode issue, got: %v", resp.Issues)
	}
}

func TestRepair_MissingSudocodeDir(t *testing.T) {
	configDir := t.TempDir()
	s := NewStore(configDir)
	old := store
	SetStore(s)
	defer func() { store = old }()

	// Register a project with a non-existent sudocodeDir
	cfg := defaultConfig()
	cfg.Projects["ghost-12345678"] = ProjectInfo{
		ID:           "ghost-12345678",
		Name:         "ghost",
		SudocodeDir:  filepath.Join(t.TempDir(), "nonexistent"),
		RegisteredAt: "2025-01-01T00:00:00Z",
		LastOpenedAt: "2025-01-01T00:00:00Z",
	}
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Dry run
	resp, err := Repair(context.Background(), &RepairParams{Fix: false})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range resp.Issues {
		if issue.Type == "sudocode_dir_missing" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected sudocode_dir_missing issue, got: %v", resp.Issues)
	}

	// Fix mode — should remove from registry
	resp, err = Repair(context.Background(), &RepairParams{Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	foundAction := false
	for _, a := range resp.Actions {
		if a.Type == "removed_from_registry" {
			foundAction = true
			break
		}
	}
	if !foundAction {
		t.Errorf("expected removed_from_registry action, got: %v", resp.Actions)
	}

	// Verify removal
	cfg2, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := cfg2.Projects["ghost-12345678"]; exists {
		t.Error("ghost project should have been removed from registry")
	}
}

func TestRepair_Rebuild(t *testing.T) {
	configDir := t.TempDir()
	s := NewStore(configDir)
	old := store
	SetStore(s)
	defer func() { store = old }()

	_, _, projectID := setupRepairProject(t, configDir, true)

	// Add a ghost project to registry
	cfg, _ := s.Load()
	cfg.Projects["ghost-12345678"] = ProjectInfo{
		ID:           "ghost-12345678",
		Name:         "ghost",
		SudocodeDir:  filepath.Join(t.TempDir(), "nonexistent"),
		RegisteredAt: "2025-01-01T00:00:00Z",
	}
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Rebuild should keep valid project, remove ghost
	resp, err := Repair(context.Background(), &RepairParams{Rebuild: true})
	if err != nil {
		t.Fatal(err)
	}

	cfg2, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := cfg2.Projects[projectID]; !exists {
		t.Error("valid project should still be in registry after rebuild")
	}
	if _, exists := cfg2.Projects["ghost-12345678"]; exists {
		t.Error("ghost project should be removed after rebuild")
	}
	if resp.Rebuilt != true {
		t.Error("expected rebuilt=true")
	}
}
