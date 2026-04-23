package jsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"encore.app/sudocode-go/internal/models"
)

// ReadOptions configures JSONL reading behaviour.
type ReadOptions struct {
	// SkipErrors silently skips malformed lines instead of returning an error.
	SkipErrors bool
}

// ReadSpecs reads a specs.jsonl file and returns the parsed specs sorted by
// created_at (ascending), then by id as a tiebreaker.
func ReadSpecs(path string, opts *ReadOptions) ([]models.SpecJSONL, error) {
	if opts == nil {
		opts = &ReadOptions{}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("jsonl: open specs file: %w", err)
	}
	defer f.Close()

	var specs []models.SpecJSONL
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10 MB max line
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var s models.SpecJSONL
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			if opts.SkipErrors {
				continue
			}
			return nil, fmt.Errorf("jsonl: parse specs line %d: %w", lineNum, err)
		}
		// Ensure non-nil slices for consistency.
		if s.Relationships == nil {
			s.Relationships = []models.RelationshipJSONL{}
		}
		if s.Tags == nil {
			s.Tags = []string{}
		}
		specs = append(specs, s)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("jsonl: scan specs file: %w", err)
	}

	sortSpecs(specs)
	return specs, nil
}

// ReadIssues reads an issues.jsonl file and returns the parsed issues sorted by
// created_at (ascending), then by id as a tiebreaker.
func ReadIssues(path string, opts *ReadOptions) ([]models.IssueJSONL, error) {
	if opts == nil {
		opts = &ReadOptions{}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("jsonl: open issues file: %w", err)
	}
	defer f.Close()

	var issues []models.IssueJSONL
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var issue models.IssueJSONL
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			if opts.SkipErrors {
				continue
			}
			return nil, fmt.Errorf("jsonl: parse issues line %d: %w", lineNum, err)
		}
		if issue.Relationships == nil {
			issue.Relationships = []models.RelationshipJSONL{}
		}
		if issue.Tags == nil {
			issue.Tags = []string{}
		}
		issues = append(issues, issue)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("jsonl: scan issues file: %w", err)
	}

	sortIssues(issues)
	return issues, nil
}

func sortSpecs(specs []models.SpecJSONL) {
	sort.SliceStable(specs, func(i, j int) bool {
		if specs[i].CreatedAt != specs[j].CreatedAt {
			return specs[i].CreatedAt < specs[j].CreatedAt
		}
		return specs[i].ID < specs[j].ID
	})
}

func sortIssues(issues []models.IssueJSONL) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].CreatedAt != issues[j].CreatedAt {
			return issues[i].CreatedAt < issues[j].CreatedAt
		}
		return issues[i].ID < issues[j].ID
	})
}
