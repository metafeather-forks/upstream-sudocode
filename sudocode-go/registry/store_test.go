package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	syncpkg "encore.app/sudocode-go/sync"
)

// writeV1Config writes a v1-format projects.json to dir.
func writeV1Config(t *testing.T, dir string, projects map[string]map[string]interface{}) {
	t.Helper()
	cfg := map[string]interface{}{
		"version":        1,
		"projects":       projects,
		"recentProjects": []string{},
		"settings": map[string]interface{}{
			"maxRecentProjects":   10,
			"autoOpenLastProject": false,
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "projects.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateV1ToV2_CoLocated(t *testing.T) {
	tmp := t.TempDir()

	// Create a fake project dir with co-located .sudocode/
	projectPath := filepath.Join(tmp, "myrepo")
	sudocodeDir := filepath.Join(projectPath, ".sudocode")
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write initial config.json (no projectId yet)
	if err := syncpkg.WriteConfig(sudocodeDir, syncpkg.ProjectConfig{SourceOfTruth: syncpkg.StorageModeJSONL}); err != nil {
		t.Fatal(err)
	}

	// Write v1 projects.json
	registryDir := filepath.Join(tmp, "config")
	writeV1Config(t, registryDir, map[string]map[string]interface{}{
		"myrepo-abc12345": {
			"id":           "myrepo-abc12345",
			"name":         "myrepo",
			"path":         projectPath,
			"sudocodeDir":  sudocodeDir,
			"registeredAt": "2024-01-01T00:00:00Z",
			"lastOpenedAt": "2024-01-01T00:00:00Z",
			"favorite":     false,
		},
	})

	store := NewStore(registryDir)
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Version should be bumped
	if cfg.Version != 2 {
		t.Errorf("expected version 2, got %d", cfg.Version)
	}

	// config.json should have projectId
	projCfg, err := syncpkg.ReadConfig(sudocodeDir)
	if err != nil {
		t.Fatal(err)
	}
	if projCfg.ProjectID != "myrepo-abc12345" {
		t.Errorf("expected projectId=myrepo-abc12345, got %q", projCfg.ProjectID)
	}

	// config.local.json should have projectdir
	localCfg, err := syncpkg.ReadLocalConfig(sudocodeDir)
	if err != nil {
		t.Fatal(err)
	}
	if localCfg.ProjectDir != projectPath {
		t.Errorf("expected projectdir=%s, got %q", projectPath, localCfg.ProjectDir)
	}

	// No .sudocode file should be created (it's already a directory)
	info, err := os.Stat(filepath.Join(projectPath, ".sudocode"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("expected .sudocode to remain a directory, not a file")
	}
}

func TestMigrateV1ToV2_External(t *testing.T) {
	tmp := t.TempDir()

	// Create project dir (no .sudocode inside it)
	projectPath := filepath.Join(tmp, "myrepo")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create external sudocode dir
	sudocodeDir := filepath.Join(tmp, "shared", ".sudocode", "myrepo")
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syncpkg.WriteConfig(sudocodeDir, syncpkg.ProjectConfig{SourceOfTruth: syncpkg.StorageModeJSONL}); err != nil {
		t.Fatal(err)
	}

	// Write v1 projects.json
	registryDir := filepath.Join(tmp, "config")
	writeV1Config(t, registryDir, map[string]map[string]interface{}{
		"myrepo-def45678": {
			"id":           "myrepo-def45678",
			"name":         "myrepo",
			"path":         projectPath,
			"sudocodeDir":  sudocodeDir,
			"registeredAt": "2024-01-01T00:00:00Z",
			"lastOpenedAt": "2024-01-01T00:00:00Z",
			"favorite":     false,
		},
	})

	store := NewStore(registryDir)
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}

	// config.json should have projectId
	projCfg, err := syncpkg.ReadConfig(sudocodeDir)
	if err != nil {
		t.Fatal(err)
	}
	if projCfg.ProjectID != "myrepo-def45678" {
		t.Errorf("expected projectId=myrepo-def45678, got %q", projCfg.ProjectID)
	}

	// A .sudocode file should be created in the project dir
	data, err := os.ReadFile(filepath.Join(projectPath, ".sudocode"))
	if err != nil {
		t.Fatal("expected .sudocode file to be created:", err)
	}
	expected := "sudocodedir: " + sudocodeDir + "\n"
	if string(data) != expected {
		t.Errorf("expected .sudocode content %q, got %q", expected, string(data))
	}
}

func TestMigrateV1ToV2_MissingSudocodeDir(t *testing.T) {
	tmp := t.TempDir()

	// Write v1 projects.json pointing to non-existent sudocodeDir
	registryDir := filepath.Join(tmp, "config")
	writeV1Config(t, registryDir, map[string]map[string]interface{}{
		"ghost-aaa11111": {
			"id":           "ghost-aaa11111",
			"name":         "ghost",
			"path":         filepath.Join(tmp, "nonexistent"),
			"sudocodeDir":  filepath.Join(tmp, "nonexistent", ".sudocode"),
			"registeredAt": "2024-01-01T00:00:00Z",
			"lastOpenedAt": "2024-01-01T00:00:00Z",
			"favorite":     false,
		},
	})

	store := NewStore(registryDir)
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Should still bump version (graceful skip)
	if cfg.Version != 2 {
		t.Errorf("expected version 2, got %d", cfg.Version)
	}
}

func TestMigrateV1ToV2_Idempotent(t *testing.T) {
	tmp := t.TempDir()

	projectPath := filepath.Join(tmp, "myrepo")
	sudocodeDir := filepath.Join(projectPath, ".sudocode")
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syncpkg.WriteConfig(sudocodeDir, syncpkg.ProjectConfig{SourceOfTruth: syncpkg.StorageModeJSONL}); err != nil {
		t.Fatal(err)
	}

	registryDir := filepath.Join(tmp, "config")
	writeV1Config(t, registryDir, map[string]map[string]interface{}{
		"myrepo-abc12345": {
			"id":           "myrepo-abc12345",
			"name":         "myrepo",
			"path":         projectPath,
			"sudocodeDir":  sudocodeDir,
			"registeredAt": "2024-01-01T00:00:00Z",
			"lastOpenedAt": "2024-01-01T00:00:00Z",
			"favorite":     false,
		},
	})

	store := NewStore(registryDir)

	// First load — triggers migration
	cfg1, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg1.Version != 2 {
		t.Fatalf("expected version 2, got %d", cfg1.Version)
	}

	// Second load — should be v2 already, no-op
	cfg2, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Version != 2 {
		t.Errorf("expected version 2 on re-load, got %d", cfg2.Version)
	}

	// Verify config.json still has the projectId (not overwritten)
	projCfg, err := syncpkg.ReadConfig(sudocodeDir)
	if err != nil {
		t.Fatal(err)
	}
	if projCfg.ProjectID != "myrepo-abc12345" {
		t.Errorf("expected projectId=myrepo-abc12345, got %q", projCfg.ProjectID)
	}
}

func TestMigrateV1ToV2_ExistingProjectIdNotOverwritten(t *testing.T) {
	tmp := t.TempDir()

	projectPath := filepath.Join(tmp, "myrepo")
	sudocodeDir := filepath.Join(projectPath, ".sudocode")
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-set a different projectId
	if err := syncpkg.WriteConfig(sudocodeDir, syncpkg.ProjectConfig{
		SourceOfTruth: syncpkg.StorageModeJSONL,
		ProjectID:     "already-set-id",
	}); err != nil {
		t.Fatal(err)
	}

	registryDir := filepath.Join(tmp, "config")
	writeV1Config(t, registryDir, map[string]map[string]interface{}{
		"myrepo-abc12345": {
			"id":           "myrepo-abc12345",
			"name":         "myrepo",
			"path":         projectPath,
			"sudocodeDir":  sudocodeDir,
			"registeredAt": "2024-01-01T00:00:00Z",
			"lastOpenedAt": "2024-01-01T00:00:00Z",
			"favorite":     false,
		},
	})

	store := NewStore(registryDir)
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}

	// Should NOT overwrite existing projectId
	projCfg, err := syncpkg.ReadConfig(sudocodeDir)
	if err != nil {
		t.Fatal(err)
	}
	if projCfg.ProjectID != "already-set-id" {
		t.Errorf("expected projectId=already-set-id (preserved), got %q", projCfg.ProjectID)
	}
}
