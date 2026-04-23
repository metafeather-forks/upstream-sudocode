package jsonl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"encore.app/sudocode-go/internal/models"
)

// WriteSpecs writes specs to a JSONL file using atomic rename.
// Entities are sorted by created_at ascending, then id as tiebreaker.
// The file mtime is set to the newest updated_at among all entities.
func WriteSpecs(path string, specs []models.SpecJSONL) error {
	sorted := make([]models.SpecJSONL, len(specs))
	copy(sorted, specs)
	sortSpecs(sorted)

	lines := make([]string, 0, len(sorted))
	var newest time.Time
	for i := range sorted {
		// Ensure non-nil slices so JSON output matches TS ([] not null).
		if sorted[i].Relationships == nil {
			sorted[i].Relationships = []models.RelationshipJSONL{}
		}
		if sorted[i].Tags == nil {
			sorted[i].Tags = []string{}
		}
		b, err := json.Marshal(&sorted[i])
		if err != nil {
			return fmt.Errorf("jsonl: marshal spec %s: %w", sorted[i].ID, err)
		}
		lines = append(lines, string(b))

		if t := parseTime(sorted[i].UpdatedAt); t.After(newest) {
			newest = t
		}
	}

	return atomicWrite(path, lines, newest)
}

// WriteIssues writes issues to a JSONL file using atomic rename.
func WriteIssues(path string, issues []models.IssueJSONL) error {
	sorted := make([]models.IssueJSONL, len(issues))
	copy(sorted, issues)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt != sorted[j].CreatedAt {
			return sorted[i].CreatedAt < sorted[j].CreatedAt
		}
		return sorted[i].ID < sorted[j].ID
	})

	lines := make([]string, 0, len(sorted))
	var newest time.Time
	for i := range sorted {
		if sorted[i].Relationships == nil {
			sorted[i].Relationships = []models.RelationshipJSONL{}
		}
		if sorted[i].Tags == nil {
			sorted[i].Tags = []string{}
		}
		b, err := json.Marshal(&sorted[i])
		if err != nil {
			return fmt.Errorf("jsonl: marshal issue %s: %w", sorted[i].ID, err)
		}
		lines = append(lines, string(b))

		if t := parseTime(sorted[i].UpdatedAt); t.After(newest) {
			newest = t
		}
	}

	return atomicWrite(path, lines, newest)
}

// atomicWrite writes lines to a temp file then renames into place.
func atomicWrite(path string, lines []string, mtime time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("jsonl: mkdir: %w", err)
	}

	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}

	// Skip write if content is unchanged.
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		return nil
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("jsonl: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("jsonl: rename: %w", err)
	}

	// Set mtime to newest updated_at.
	if !mtime.IsZero() {
		_ = os.Chtimes(path, mtime, mtime)
	}

	return nil
}

// parseTime tries multiple time formats used in the JSONL files.
func parseTime(s string) time.Time {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
