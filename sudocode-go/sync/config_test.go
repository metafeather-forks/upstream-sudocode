package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectConfig_ProjectID(t *testing.T) {
	dir := t.TempDir()

	cfg := ProjectConfig{
		SourceOfTruth: StorageModeJSONL,
		ProjectID:     "vendor-sudocode-b20d41b9",
	}
	if err := WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}

	got, err := ReadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != "vendor-sudocode-b20d41b9" {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, "vendor-sudocode-b20d41b9")
	}

	// Verify JSON format
	data, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if raw["projectId"] != "vendor-sudocode-b20d41b9" {
		t.Errorf("JSON key projectId = %v", raw["projectId"])
	}
}

func TestLocalConfig_ReadWrite(t *testing.T) {
	dir := t.TempDir()

	cfg := LocalConfig{ProjectDir: "/home/user/my-project"}
	if err := WriteLocalConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectDir != "/home/user/my-project" {
		t.Errorf("ProjectDir = %q, want %q", got.ProjectDir, "/home/user/my-project")
	}

	// Verify JSON format
	data, _ := os.ReadFile(filepath.Join(dir, "config.local.json"))
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if raw["projectdir"] != "/home/user/my-project" {
		t.Errorf("JSON key projectdir = %v", raw["projectdir"])
	}
}

func TestLocalConfig_NotFound(t *testing.T) {
	dir := t.TempDir()

	got, err := ReadLocalConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectDir != "" {
		t.Errorf("expected empty ProjectDir, got %q", got.ProjectDir)
	}
}
