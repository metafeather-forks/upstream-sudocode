package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// StorageMode determines the source of truth for project data.
type StorageMode string

const (
	// StorageModeJSONL uses JSONL files as the source of truth.
	StorageModeJSONL StorageMode = "jsonl"
	// StorageModeMarkdown uses markdown files as the source of truth.
	StorageModeMarkdown StorageMode = "markdown"
)

// ProjectConfig represents the .sudocode/config.json file.
type ProjectConfig struct {
	SourceOfTruth StorageMode `json:"sourceOfTruth,omitempty"`
}

// DefaultConfig returns a ProjectConfig with default values.
func DefaultConfig() ProjectConfig {
	return ProjectConfig{SourceOfTruth: StorageModeJSONL}
}

// ReadConfig reads the config.json from a .sudocode/ directory.
func ReadConfig(sudocodeDir string) (ProjectConfig, error) {
	path := filepath.Join(sudocodeDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return ProjectConfig{}, fmt.Errorf("sync: read config: %w", err)
	}

	var cfg ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ProjectConfig{}, fmt.Errorf("sync: parse config: %w", err)
	}

	if cfg.SourceOfTruth == "" {
		cfg.SourceOfTruth = StorageModeJSONL
	}
	return cfg, nil
}

// WriteConfig writes config.json to a .sudocode/ directory.
func WriteConfig(sudocodeDir string, cfg ProjectConfig) error {
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		return fmt.Errorf("sync: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("sync: marshal config: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(sudocodeDir, "config.json"), data, 0o644)
}

// IsMarkdownFirst returns true when markdown files are the source of truth.
func (c ProjectConfig) IsMarkdownFirst() bool {
	return c.SourceOfTruth == StorageModeMarkdown
}

// GitignoreContent returns the .gitignore content appropriate for the storage mode.
// In JSONL mode, markdown files are ignored (they're derived).
// In markdown mode, JSONL files are ignored (they're derived).
// config.json is never ignored.
func GitignoreContent(mode StorageMode) string {
	switch mode {
	case StorageModeMarkdown:
		return "# Auto-generated: markdown is source of truth\nspecs.jsonl\nissues.jsonl\n"
	default: // jsonl
		return "# Auto-generated: JSONL is source of truth\nspecs/\nissues/\n"
	}
}

// WriteGitignore writes the .gitignore file inside the .sudocode/ directory.
func WriteGitignore(sudocodeDir string, mode StorageMode) error {
	path := filepath.Join(sudocodeDir, ".gitignore")
	return os.WriteFile(path, []byte(GitignoreContent(mode)), 0o644)
}

// ConflictResolution determines whether to prefer file or DB data during import.
type ConflictResolution int

const (
	// PreferFile uses file data when timestamps cannot determine a winner.
	PreferFile ConflictResolution = iota
	// PreferDB keeps database data when timestamps cannot determine a winner.
	PreferDB
)

// ResolveConflict returns which side to prefer based on the config sourceOfTruth.
// When sourceOfTruth is "jsonl" or "markdown" (file-based), files win ties.
// This is a simple last-write-wins strategy placeholder; more sophisticated
// three-way merge is left to higher-level callers.
func ResolveConflict(cfg ProjectConfig) ConflictResolution {
	// File-based source of truth always prefers files on conflict.
	return PreferFile
}
