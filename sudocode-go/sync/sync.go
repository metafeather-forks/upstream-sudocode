package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"encore.app/sudocode-go/internal/jsonl"
	"encore.app/sudocode-go/internal/markdown"
	"encore.app/sudocode-go/internal/models"
)

// ImportResult holds the entities parsed from a .sudocode/ directory.
type ImportResult struct {
	Specs  []models.SpecJSONL
	Issues []models.IssueJSONL
	Config ProjectConfig
}

// Import reads a .sudocode/ directory and returns all entities found.
// It respects the sourceOfTruth setting in config.json:
//   - "jsonl" (default): reads from specs.jsonl / issues.jsonl
//   - "markdown": reads from specs/*.md / issues/*.md
//
// JSONL is always read as a fallback when markdown files are absent.
func Import(sudocodeDir string) (*ImportResult, error) {
	cfg, err := ReadConfig(sudocodeDir)
	if err != nil {
		return nil, fmt.Errorf("sync: import config: %w", err)
	}

	result := &ImportResult{Config: cfg}

	if cfg.IsMarkdownFirst() {
		result.Specs, err = importSpecsFromMarkdown(sudocodeDir)
		if err != nil {
			return nil, err
		}
		result.Issues, err = importIssuesFromMarkdown(sudocodeDir)
		if err != nil {
			return nil, err
		}
	} else {
		result.Specs, err = importSpecsFromJSONL(sudocodeDir)
		if err != nil {
			return nil, err
		}
		result.Issues, err = importIssuesFromJSONL(sudocodeDir)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// Export writes entities to a .sudocode/ directory.
// JSONL files are always written. Markdown files are written based on config.
// The .gitignore and config.json are also written.
func Export(sudocodeDir string, specs []models.SpecJSONL, issues []models.IssueJSONL, cfg ProjectConfig) error {
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		return fmt.Errorf("sync: mkdir: %w", err)
	}

	// Always write config.
	if err := WriteConfig(sudocodeDir, cfg); err != nil {
		return err
	}

	// Always write JSONL (regardless of sourceOfTruth mode).
	if err := exportJSONL(sudocodeDir, specs, issues); err != nil {
		return err
	}

	// Write markdown files.
	if err := exportMarkdown(sudocodeDir, specs, issues); err != nil {
		return err
	}

	// Write .gitignore based on mode.
	if err := WriteGitignore(sudocodeDir, cfg.SourceOfTruth); err != nil {
		return fmt.Errorf("sync: write gitignore: %w", err)
	}

	return nil
}

// ExportSpecs writes only specs to the .sudocode/ directory.
func ExportSpecs(sudocodeDir string, specs []models.SpecJSONL, cfg ProjectConfig) error {
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		return fmt.Errorf("sync: mkdir: %w", err)
	}

	specsPath := filepath.Join(sudocodeDir, "specs.jsonl")
	if err := jsonl.WriteSpecs(specsPath, specs); err != nil {
		return fmt.Errorf("sync: write specs jsonl: %w", err)
	}

	specsDir := filepath.Join(sudocodeDir, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return fmt.Errorf("sync: mkdir specs: %w", err)
	}
	for _, s := range specs {
		if err := writeSpecMarkdown(specsDir, s); err != nil {
			return err
		}
	}

	return nil
}

// ExportIssues writes only issues to the .sudocode/ directory.
func ExportIssues(sudocodeDir string, issues []models.IssueJSONL, cfg ProjectConfig) error {
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		return fmt.Errorf("sync: mkdir: %w", err)
	}

	issuesPath := filepath.Join(sudocodeDir, "issues.jsonl")
	if err := jsonl.WriteIssues(issuesPath, issues); err != nil {
		return fmt.Errorf("sync: write issues jsonl: %w", err)
	}

	issuesDir := filepath.Join(sudocodeDir, "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		return fmt.Errorf("sync: mkdir issues: %w", err)
	}
	for _, iss := range issues {
		if err := writeIssueMarkdown(issuesDir, iss); err != nil {
			return err
		}
	}

	return nil
}

// --- JSONL import ---

func importSpecsFromJSONL(sudocodeDir string) ([]models.SpecJSONL, error) {
	path := filepath.Join(sudocodeDir, "specs.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []models.SpecJSONL{}, nil
	}
	specs, err := jsonl.ReadSpecs(path, &jsonl.ReadOptions{SkipErrors: true})
	if err != nil {
		return nil, fmt.Errorf("sync: read specs jsonl: %w", err)
	}
	if specs == nil {
		specs = []models.SpecJSONL{}
	}
	return specs, nil
}

func importIssuesFromJSONL(sudocodeDir string) ([]models.IssueJSONL, error) {
	path := filepath.Join(sudocodeDir, "issues.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []models.IssueJSONL{}, nil
	}
	issues, err := jsonl.ReadIssues(path, &jsonl.ReadOptions{SkipErrors: true})
	if err != nil {
		return nil, fmt.Errorf("sync: read issues jsonl: %w", err)
	}
	if issues == nil {
		issues = []models.IssueJSONL{}
	}
	return issues, nil
}

// --- Markdown import ---

func importSpecsFromMarkdown(sudocodeDir string) ([]models.SpecJSONL, error) {
	specsDir := filepath.Join(sudocodeDir, "specs")
	if _, err := os.Stat(specsDir); os.IsNotExist(err) {
		// Fall back to JSONL if no markdown dir exists.
		return importSpecsFromJSONL(sudocodeDir)
	}

	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return nil, fmt.Errorf("sync: read specs dir: %w", err)
	}

	var specs []models.SpecJSONL
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("sync: read spec %s: %w", entry.Name(), err)
		}
		sf, err := markdown.ParseSpec(data)
		if err != nil {
			continue // skip malformed files
		}
		specs = append(specs, sf.Spec)
	}
	if specs == nil {
		specs = []models.SpecJSONL{}
	}
	return specs, nil
}

func importIssuesFromMarkdown(sudocodeDir string) ([]models.IssueJSONL, error) {
	issuesDir := filepath.Join(sudocodeDir, "issues")
	if _, err := os.Stat(issuesDir); os.IsNotExist(err) {
		return importIssuesFromJSONL(sudocodeDir)
	}

	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		return nil, fmt.Errorf("sync: read issues dir: %w", err)
	}

	var issues []models.IssueJSONL
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(issuesDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("sync: read issue %s: %w", entry.Name(), err)
		}
		isf, err := markdown.ParseIssue(data)
		if err != nil {
			continue
		}
		issues = append(issues, isf.Issue)
	}
	if issues == nil {
		issues = []models.IssueJSONL{}
	}
	return issues, nil
}

// --- JSONL export ---

func exportJSONL(sudocodeDir string, specs []models.SpecJSONL, issues []models.IssueJSONL) error {
	specsPath := filepath.Join(sudocodeDir, "specs.jsonl")
	if err := jsonl.WriteSpecs(specsPath, specs); err != nil {
		return fmt.Errorf("sync: write specs jsonl: %w", err)
	}

	issuesPath := filepath.Join(sudocodeDir, "issues.jsonl")
	if err := jsonl.WriteIssues(issuesPath, issues); err != nil {
		return fmt.Errorf("sync: write issues jsonl: %w", err)
	}

	return nil
}

// --- Markdown export ---

func exportMarkdown(sudocodeDir string, specs []models.SpecJSONL, issues []models.IssueJSONL) error {
	specsDir := filepath.Join(sudocodeDir, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return fmt.Errorf("sync: mkdir specs: %w", err)
	}
	for _, s := range specs {
		if err := writeSpecMarkdown(specsDir, s); err != nil {
			return err
		}
	}

	issuesDir := filepath.Join(sudocodeDir, "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		return fmt.Errorf("sync: mkdir issues: %w", err)
	}
	for _, iss := range issues {
		if err := writeIssueMarkdown(issuesDir, iss); err != nil {
			return err
		}
	}

	return nil
}

func writeSpecMarkdown(dir string, s models.SpecJSONL) error {
	sf := &markdown.SpecFile{
		Spec: s,
		Body: s.Content,
	}
	data, err := markdown.MarshalSpec(sf)
	if err != nil {
		return fmt.Errorf("sync: marshal spec %s: %w", s.ID, err)
	}
	filename := markdown.SpecFilename(s.ID, s.Title)
	return os.WriteFile(filepath.Join(dir, filename), data, 0o644)
}

func writeIssueMarkdown(dir string, iss models.IssueJSONL) error {
	isf := &markdown.IssueFile{
		Issue: iss,
		Body:  iss.Content,
	}
	data, err := markdown.MarshalIssue(isf)
	if err != nil {
		return fmt.Errorf("sync: marshal issue %s: %w", iss.ID, err)
	}
	filename := markdown.IssueFilename(iss.ID, iss.Title)
	return os.WriteFile(filepath.Join(dir, filename), data, 0o644)
}
