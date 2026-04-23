package markdown

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"encore.app/sudocode-go/internal/models"
)

// FrontmatterRelationship is the YAML representation of a relationship in frontmatter.
type FrontmatterRelationship struct {
	ToID             string `yaml:"to_id"`
	ToType           string `yaml:"to_type"`
	RelationshipType string `yaml:"relationship_type"`
}

// SpecFile represents a parsed spec markdown file.
type SpecFile struct {
	Spec models.SpecJSONL
	Body string
}

// IssueFile represents a parsed issue markdown file.
type IssueFile struct {
	Issue models.IssueJSONL
	Body  string
}

// specFrontmatter is the intermediate YAML structure for spec files.
type specFrontmatter struct {
	ID            string                    `yaml:"id"`
	Title         string                    `yaml:"title"`
	Priority      int                       `yaml:"priority"`
	CreatedAt     string                    `yaml:"created_at"`
	ParentID      *string                   `yaml:"parent_id,omitempty"`
	Tags          []string                  `yaml:"tags,omitempty"`
	Relationships []FrontmatterRelationship `yaml:"relationships,omitempty"`
}

// issueFrontmatter is the intermediate YAML structure for issue files.
type issueFrontmatter struct {
	ID            string                    `yaml:"id"`
	Title         string                    `yaml:"title"`
	Priority      int                       `yaml:"priority"`
	CreatedAt     string                    `yaml:"created_at"`
	ParentID      *string                   `yaml:"parent_id,omitempty"`
	Tags          []string                  `yaml:"tags,omitempty"`
	Relationships []FrontmatterRelationship `yaml:"relationships,omitempty"`
	Status        string                    `yaml:"status"`
	Assignee      *string                   `yaml:"assignee,omitempty"`
	ClosedAt      *string                   `yaml:"closed_at,omitempty"`
}

const frontmatterDelimiter = "---"

// splitFrontmatter splits raw markdown bytes into YAML frontmatter and body.
// Returns empty frontmatter if no valid frontmatter delimiter is found.
func splitFrontmatter(data []byte) (yamlBytes []byte, body string, err error) {
	s := string(data)
	if !strings.HasPrefix(s, frontmatterDelimiter+"\n") {
		return nil, s, nil
	}
	rest := s[len(frontmatterDelimiter)+1:]
	idx := strings.Index(rest, "\n"+frontmatterDelimiter+"\n")
	if idx < 0 {
		return nil, s, nil
	}
	yamlPart := rest[:idx]
	bodyPart := rest[idx+len("\n"+frontmatterDelimiter+"\n"):]
	return []byte(yamlPart), bodyPart, nil
}

// ParseSpec parses a spec markdown file from raw bytes.
func ParseSpec(data []byte) (*SpecFile, error) {
	yamlBytes, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	if yamlBytes == nil {
		return nil, fmt.Errorf("markdown: no frontmatter found")
	}

	var fm specFrontmatter
	if err := yaml.Unmarshal(yamlBytes, &fm); err != nil {
		return nil, fmt.Errorf("markdown: parse spec frontmatter: %w", err)
	}

	spec := models.SpecJSONL{
		Spec: models.Spec{
			ID:        fm.ID,
			Title:     fm.Title,
			Priority:  fm.Priority,
			CreatedAt: fm.CreatedAt,
			ParentID:  fm.ParentID,
			Content:   body,
		},
		Tags: fm.Tags,
	}

	for _, r := range fm.Relationships {
		spec.Relationships = append(spec.Relationships, models.RelationshipJSONL{
			From:     fm.ID,
			FromType: models.EntityTypeSpec,
			To:       r.ToID,
			ToType:   models.EntityType(r.ToType),
			Type:     models.RelationshipType(r.RelationshipType),
		})
	}

	return &SpecFile{Spec: spec, Body: body}, nil
}

// ParseIssue parses an issue markdown file from raw bytes.
func ParseIssue(data []byte) (*IssueFile, error) {
	yamlBytes, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	if yamlBytes == nil {
		return nil, fmt.Errorf("markdown: no frontmatter found")
	}

	var fm issueFrontmatter
	if err := yaml.Unmarshal(yamlBytes, &fm); err != nil {
		return nil, fmt.Errorf("markdown: parse issue frontmatter: %w", err)
	}

	issue := models.IssueJSONL{
		Issue: models.Issue{
			ID:        fm.ID,
			Title:     fm.Title,
			Priority:  fm.Priority,
			Status:    models.IssueStatus(fm.Status),
			CreatedAt: fm.CreatedAt,
			ParentID:  fm.ParentID,
			Assignee:  fm.Assignee,
			ClosedAt:  fm.ClosedAt,
			Content:   body,
		},
		Tags: fm.Tags,
	}

	for _, r := range fm.Relationships {
		issue.Relationships = append(issue.Relationships, models.RelationshipJSONL{
			From:     fm.ID,
			FromType: models.EntityTypeIssue,
			To:       r.ToID,
			ToType:   models.EntityType(r.ToType),
			Type:     models.RelationshipType(r.RelationshipType),
		})
	}

	return &IssueFile{Issue: issue, Body: body}, nil
}

// MarshalSpec serializes a spec to markdown bytes with YAML frontmatter.
func MarshalSpec(sf *SpecFile) ([]byte, error) {
	fm := specFrontmatter{
		ID:        sf.Spec.ID,
		Title:     sf.Spec.Title,
		Priority:  sf.Spec.Priority,
		CreatedAt: sf.Spec.CreatedAt,
		ParentID:  sf.Spec.ParentID,
	}
	if len(sf.Spec.Tags) > 0 {
		fm.Tags = sf.Spec.Tags
	}
	for _, r := range sf.Spec.Relationships {
		fm.Relationships = append(fm.Relationships, FrontmatterRelationship{
			ToID:             r.To,
			ToType:           string(r.ToType),
			RelationshipType: string(r.Type),
		})
	}
	return marshalFrontmatter(fm, sf.Body)
}

// MarshalIssue serializes an issue to markdown bytes with YAML frontmatter.
func MarshalIssue(isf *IssueFile) ([]byte, error) {
	fm := issueFrontmatter{
		ID:        isf.Issue.ID,
		Title:     isf.Issue.Title,
		Priority:  isf.Issue.Priority,
		CreatedAt: isf.Issue.CreatedAt,
		ParentID:  isf.Issue.ParentID,
		Status:    string(isf.Issue.Status),
		Assignee:  isf.Issue.Assignee,
		ClosedAt:  isf.Issue.ClosedAt,
	}
	if len(isf.Issue.Tags) > 0 {
		fm.Tags = isf.Issue.Tags
	}
	for _, r := range isf.Issue.Relationships {
		fm.Relationships = append(fm.Relationships, FrontmatterRelationship{
			ToID:             r.To,
			ToType:           string(r.ToType),
			RelationshipType: string(r.Type),
		})
	}
	return marshalFrontmatter(fm, isf.Body)
}

func marshalFrontmatter(fm interface{}, body string) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("markdown: marshal frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString(frontmatterDelimiter + "\n")
	buf.Write(yamlBytes)
	buf.WriteString(frontmatterDelimiter + "\n")
	buf.WriteString(body)
	return buf.Bytes(), nil
}

// Slugify converts a title to a URL-friendly slug.
func Slugify(title string) string {
	s := strings.ToLower(title)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// collapse multiple hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	return s
}

// SpecFilename returns the canonical filename for a spec, e.g. "s-14sh_my-spec.md".
func SpecFilename(id, title string) string {
	return id + "_" + Slugify(title) + ".md"
}

// IssueFilename returns the canonical filename for an issue, e.g. "i-x7k9_fix-bug.md".
func IssueFilename(id, title string) string {
	return id + "_" + Slugify(title) + ".md"
}

// NowISO returns the current time in ISO 8601 format matching the TS implementation.
func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
