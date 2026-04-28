# sudocode-go: Encore Go Codebase Walkthrough

*2026-04-27T07:17:44Z by Showboat 0.6.1*
<!-- showboat-id: cb7126b4-61bd-45d5-845a-88bcfff90af4 -->

This is a linear walkthrough of the `sudocode-go` codebase — an Encore Go port of the Sudocode project management system. Sudocode provides specs, issues, relationships, and feedback as structured data that AI coding agents interact with via MCP (Model Context Protocol).

The Go port is a drop-in replacement for the existing TypeScript implementation, maintaining full backwards compatibility with `.sudocode/` file formats and the `~/.config/sudocode/projects.json` registry.

## Architecture at a Glance

The codebase is ~7,100 lines of Go across 53 files, with ~2,900 lines of tests (40% test coverage by line count). It follows Encore Go conventions: services are top-level directories, each with typed API endpoints.

**Request flow:** Client → Encore API (auth extracts X-Project-ID) → Service endpoint → Postgres DB → File sync → WebSocket broadcast

**Services:**
- `auth/` — Extracts project ID from headers
- `registry/` — Project lifecycle (open/close/init), persists to `projects.json`
- `projects/` — Spec/Issue/Relationship/Feedback CRUD against Postgres
- `sync/` — Bidirectional `.sudocode/` filesystem ↔ DB synchronization
- `realtime/` — WebSocket pub-sub for live updates
- `mcpserver/` — 11 MCP tools over HTTP/SSE for AI agents
- `web/` — Embedded SPA frontend serving
- `cmd/sudocode-mcp/` — stdio shim bridging MCP JSON-RPC to the HTTP daemon

## 1. Project Setup: `encore.app` and `go.mod`

The root of the Encore application is declared in two files.

```bash
cat sudocode-go/encore.app
```

```output
{
    "id": "sudocode-go"
}
```

```bash
head -10 sudocode-go/go.mod
```

```output
module encore.app/sudocode-go

go 1.26.1

require (
	encore.dev v1.52.1
	github.com/coder/websocket v1.8.13
	github.com/mark3labs/mcp-go v0.49.0
	gopkg.in/yaml.v3 v3.0.1
)
```

Key dependencies: **Encore** runtime for services/Postgres/auth, **coder/websocket** for realtime, **mark3labs/mcp-go** for the MCP protocol, and **yaml.v3** for frontmatter parsing.

## 2. Data Models: `internal/models/`

All Sudocode entities are defined as Go structs with two forms: a **DB form** (flat, for Postgres) and a **JSONL form** (with inline relationships/tags, for file serialization). This dual representation is the key to backwards compatibility with the TypeScript implementation.

```bash
cat sudocode-go/internal/models/spec.go
```

```output
package models

// Spec represents a specification entity (DB/core form).
type Spec struct {
	ID            string          `json:"id"`
	UUID          string          `json:"uuid"`
	Title         string          `json:"title"`
	FilePath      string          `json:"file_path"`
	Content       string          `json:"content"`
	Priority      int             `json:"priority"`
	Archived      *int            `json:"archived,omitempty"`
	ArchivedAt    *string         `json:"archived_at,omitempty"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
	ParentID      *string         `json:"parent_id"`
	ParentUUID    *string         `json:"parent_uuid"`
	ExternalLinks []ExternalLink  `json:"external_links,omitempty"`
}

// SpecJSONL is the JSONL serialization format for specs.
type SpecJSONL struct {
	Spec
	Relationships []RelationshipJSONL `json:"relationships"`
	Tags          []string            `json:"tags"`
}
```

```bash
cat sudocode-go/internal/models/issue.go
```

```output
package models

// IssueStatus represents the workflow status of an issue.
type IssueStatus string

const (
	IssueStatusOpen        IssueStatus = "open"
	IssueStatusInProgress  IssueStatus = "in_progress"
	IssueStatusBlocked     IssueStatus = "blocked"
	IssueStatusNeedsReview IssueStatus = "needs_review"
	IssueStatusClosed      IssueStatus = "closed"
	IssueStatusWontFix     IssueStatus = "wont_fix"
	IssueStatusDuplicate   IssueStatus = "duplicate"
)

// Issue represents an issue entity (DB/core form).
type Issue struct {
	ID            string          `json:"id"`
	UUID          string          `json:"uuid"`
	Title         string          `json:"title"`
	Status        IssueStatus     `json:"status"`
	Content       string          `json:"content"`
	Priority      int             `json:"priority"`
	Assignee      *string         `json:"assignee"`
	Archived      *int            `json:"archived,omitempty"`
	ArchivedAt    *string         `json:"archived_at,omitempty"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
	ClosedAt      *string         `json:"closed_at,omitempty"`
	ParentID      *string         `json:"parent_id"`
	ParentUUID    *string         `json:"parent_uuid"`
	ExternalLinks []ExternalLink  `json:"external_links,omitempty"`
}

// IssueJSONL is the JSONL serialization format for issues.
type IssueJSONL struct {
	Issue
	Relationships []RelationshipJSONL `json:"relationships"`
	Tags          []string            `json:"tags"`
	Feedback      []FeedbackJSONL     `json:"feedback,omitempty"`
}
```

Notice how `SpecJSONL` embeds `Spec` and adds `Relationships` and `Tags` — these get flattened into the JSONL file format. `IssueJSONL` additionally includes inline `Feedback`. The `*int` type for `Archived` and `*string` for nullable fields match the TypeScript schema where `0`/`1` integers and null strings are used.

```bash
cat sudocode-go/internal/models/feedback.go
```

```output
package models

// FeedbackType represents the kind of feedback.
type FeedbackType string

const (
	FeedbackTypeComment    FeedbackType = "comment"
	FeedbackTypeSuggestion FeedbackType = "suggestion"
	FeedbackTypeRequest    FeedbackType = "request"
)

// LocationAnchor tracks a position in a markdown document.
type LocationAnchor struct {
	SectionHeading *string `json:"section_heading,omitempty"`
	SectionLevel   *int    `json:"section_level,omitempty"`
	LineNumber     *int    `json:"line_number,omitempty"`
	LineOffset     *int    `json:"line_offset,omitempty"`
	TextSnippet    *string `json:"text_snippet,omitempty"`
	ContextBefore  *string `json:"context_before,omitempty"`
	ContextAfter   *string `json:"context_after,omitempty"`
	ContentHash    *string `json:"content_hash,omitempty"`
}

// OriginalLocation stores the original position before relocation.
type OriginalLocation struct {
	LineNumber     int     `json:"line_number"`
	SectionHeading *string `json:"section_heading,omitempty"`
}

// FeedbackAnchor extends LocationAnchor with change-tracking fields.
type FeedbackAnchor struct {
	LocationAnchor
	AnchorStatus     string            `json:"anchor_status"`
	LastVerifiedAt   *string           `json:"last_verified_at,omitempty"`
	OriginalLocation *OriginalLocation `json:"original_location,omitempty"`
}

// IssueFeedback represents feedback on a spec or issue (DB form).
type IssueFeedback struct {
	ID           string       `json:"id"`
	FromID       *string      `json:"from_id,omitempty"`
	FromUUID     *string      `json:"from_uuid,omitempty"`
	ToID         string       `json:"to_id"`
	ToUUID       string       `json:"to_uuid"`
	FeedbackType FeedbackType `json:"feedback_type"`
	Content      string       `json:"content"`
	Agent        *string      `json:"agent,omitempty"`
	Anchor       *string      `json:"anchor,omitempty"`
	Dismissed    *bool        `json:"dismissed,omitempty"`
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
}

// FeedbackJSONL is the JSONL serialization format for feedback.
type FeedbackJSONL struct {
	ID           string          `json:"id"`
	FromID       *string         `json:"from_id,omitempty"`
	ToID         string          `json:"to_id"`
	FeedbackType FeedbackType    `json:"feedback_type"`
	Content      string          `json:"content"`
	Agent        *string         `json:"agent,omitempty"`
	Anchor       *FeedbackAnchor `json:"anchor,omitempty"`
	Dismissed    *bool           `json:"dismissed,omitempty"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}
```

The feedback model is the most complex — it supports anchoring comments to specific lines/sections of a document with drift detection (`FeedbackAnchor`). This lets agents pin review comments to specific parts of a spec.

```bash
cd sudocode-go && go test ./internal/models/ -v -count=1 2>&1 | grep -E "^(=== RUN|--- |ok )"
```

```output
=== RUN   TestSpecJSONLRoundTrip
--- PASS: TestSpecJSONLRoundTrip (0.00s)
=== RUN   TestIssueJSONLRoundTrip
--- PASS: TestIssueJSONLRoundTrip (0.00s)
=== RUN   TestRelationshipJSONLFields
--- PASS: TestRelationshipJSONLFields (0.00s)
=== RUN   TestFeedbackJSONLWithAnchor
--- PASS: TestFeedbackJSONLWithAnchor (0.00s)
=== RUN   TestRelationshipDBForm
--- PASS: TestRelationshipDBForm (0.00s)
=== RUN   TestIssueFeedbackDBForm
--- PASS: TestIssueFeedbackDBForm (0.00s)
=== RUN   TestSpecJSONLOmitsNulls
--- PASS: TestSpecJSONLOmitsNulls (0.00s)
ok  	encore.app/sudocode-go/internal/models	0.194s
```

## 3. Project ID Generation: `internal/projectid/`

Sudocode identifies projects by a deterministic ID derived from the filesystem path. This must match the TypeScript implementation exactly — any difference would break cross-tool compatibility.

```bash
sed -n "1,50p" sudocode-go/internal/projectid/projectid.go
```

```output
package projectid

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"math"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	nonAlphanumDash = regexp.MustCompile(`[^a-z0-9-]`)
	multiDash       = regexp.MustCompile(`-+`)
	legacyIDRe      = regexp.MustCompile(`^(SPEC|ISSUE)-\d+$`)
	hashIDRe        = regexp.MustCompile(`^[is]-[0-9a-z]{4,8}$`)
)

// Generate produces a deterministic project ID from an absolute path.
// The result matches the TypeScript generateProjectId implementation:
//
//	safeName(basename(path)) + "-" + sha256(path)[:8]
func Generate(absolutePath string) string {
	base := filepath.Base(absolutePath)

	safe := strings.ToLower(base)
	safe = nonAlphanumDash.ReplaceAllString(safe, "-")
	safe = multiDash.ReplaceAllString(safe, "-")
	safe = strings.Trim(safe, "-")
	if len(safe) > 32 {
		safe = safe[:32]
	}

	h := sha256.Sum256([]byte(absolutePath))
	hash := fmt.Sprintf("%x", h[:])[:8]

	return safe + "-" + hash
}

// HashUUIDToBase36 converts a UUID to a base36 hash of the given length,
// matching the TypeScript hashUUIDToBase36 implementation.
func HashUUIDToBase36(uuid string, length int) string {
	clean := strings.ReplaceAll(uuid, "-", "")

	h := sha256.Sum256([]byte(clean))
	hexStr := fmt.Sprintf("%x", h[:])

	hexNeeded := int(math.Ceil(float64(length) * 1.29))
	hexSub := hexStr[:hexNeeded]
```

```bash
cd sudocode-go && go test ./internal/projectid/ -v -run TestGenerate -count=1 2>&1
```

```output
=== RUN   TestGenerate
=== RUN   TestGenerate//home/user/projects/my-app
=== RUN   TestGenerate//Users/metafeather/Developer/src/metafeather.net/metafeather-forks/vendor-sudocode
=== RUN   TestGenerate//tmp/test
=== RUN   TestGenerate//home/user/My_Cool_Project!!!
=== RUN   TestGenerate//a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/this-is-a-very-long-directory-name-that-exceeds-32-chars
--- PASS: TestGenerate (0.00s)
    --- PASS: TestGenerate//home/user/projects/my-app (0.00s)
    --- PASS: TestGenerate//Users/metafeather/Developer/src/metafeather.net/metafeather-forks/vendor-sudocode (0.00s)
    --- PASS: TestGenerate//tmp/test (0.00s)
    --- PASS: TestGenerate//home/user/My_Cool_Project!!! (0.00s)
    --- PASS: TestGenerate//a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/this-is-a-very-long-directory-name-that-exceeds-32-chars (0.00s)
PASS
ok  	encore.app/sudocode-go/internal/projectid	0.184s
```

The algorithm: `lowercase(basename) → replace non-alphanumeric with dashes → truncate to 32 → append "-" + sha256(path)[:8]`. The test cases include known outputs from the TypeScript implementation to ensure exact parity.

## 4. JSONL Reader/Writer: `internal/jsonl/`

The `.sudocode/specs.jsonl` and `.sudocode/issues.jsonl` files are the primary storage format. Each line is a self-contained JSON object. The writer uses atomic rename to prevent corruption.

```bash
cd sudocode-go && go test ./internal/jsonl/ -v -run TestTSCompatible -count=1 2>&1
```

```output
=== RUN   TestTSCompatibleFormat
--- PASS: TestTSCompatibleFormat (0.00s)
PASS
ok  	encore.app/sudocode-go/internal/jsonl	0.186s
```

The `TestTSCompatibleFormat` test verifies that Go produces JSON output the TypeScript implementation can read — field ordering, null handling, and integer-encoded booleans all match.

## 5. Markdown Frontmatter: `internal/markdown/`

Sudocode also stores specs and issues as individual markdown files with YAML frontmatter. This is the human-readable format that lives in `.sudocode/specs/` and `.sudocode/issues/`.

```bash
cd sudocode-go && go test ./internal/markdown/ -v -run TestRoundTripSpecFile -count=1 2>&1
```

```output
=== RUN   TestRoundTripSpecFile
--- PASS: TestRoundTripSpecFile (0.00s)
PASS
ok  	encore.app/sudocode-go/internal/markdown	0.190s
```

Round-trip tests write a spec to a markdown file then read it back, verifying all fields survive the YAML frontmatter → struct → YAML frontmatter cycle. Filenames follow the pattern `s-14sh_authentication-flow.md` using `Slugify()` on the title.

## 6. Auth Handler: `auth/`

Encore's auth handler extracts `X-Project-ID` from incoming HTTP requests and makes it available to all `//encore:api auth` endpoints via `auth.Data`.

```bash
cat sudocode-go/auth/auth.go
```

```output
package auth

import (
	"context"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
)

// Params defines the auth handler input extracted from request headers.
type Params struct {
	XProjectID string `header:"X-Project-ID"`
}

// Data holds the authenticated project context available to all auth endpoints.
type Data struct {
	ProjectID string
}

// AuthHandler validates the X-Project-ID header and returns auth data.
//
//encore:authhandler
func AuthHandler(ctx context.Context, params *Params) (auth.UID, *Data, error) {
	if params.XProjectID == "" {
		return "", nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "missing X-Project-ID header",
		}
	}

	return auth.UID(params.XProjectID), &Data{ProjectID: params.XProjectID}, nil
}
```

This is the multi-tenancy mechanism. Every project-scoped endpoint (`projects/` service) uses `//encore:api auth`, so the project ID is always available via `auth.Data().(*auth.Data).ProjectID`. Registry endpoints are `//encore:api public` — they manage the project list itself. Raw endpoints (WebSocket, MCP, frontend) bypass typed auth and extract the header manually.

## 7. Project Registry: `registry/`

The registry service manages project lifecycle — opening, closing, initializing new projects, and tracking recently used projects. It persists to `~/.config/sudocode/projects.json`, the same file the TypeScript implementation uses.

```bash
grep "//encore:api" sudocode-go/registry/registry.go
```

```output
//encore:api public method=POST path=/registry/list
//encore:api public method=POST path=/registry/open
//encore:api public method=POST path=/registry/close
//encore:api public method=POST path=/registry/init
//encore:api public method=POST path=/registry/validate
//encore:api public method=POST path=/registry/browse
//encore:api public method=POST path=/registry/current
//encore:api public method=POST path=/registry/set-current
//encore:api public method=POST path=/registry/recent
```

```bash
cd sudocode-go && go test ./registry/ -v -run TestOpen -count=1 2>&1
```

```output
=== RUN   TestOpen
--- PASS: TestOpen (0.00s)
=== RUN   TestOpenIdempotent
--- PASS: TestOpenIdempotent (0.00s)
PASS
ok  	encore.app/sudocode-go/registry	0.199s
```

All 9 registry endpoints are `public` (no auth) since they manage the project list itself. The `Open` endpoint generates a deterministic project ID, registers the project, sets it as current, and updates the recent list. All tests use `t.TempDir()` — never the real `~/.config/sudocode/` directory.

## 8. Projects Service: `projects/` — The CRUD Core

This is the largest service. It provides Postgres-backed CRUD for specs, issues, relationships, and feedback — all scoped by project ID via the auth handler.

```bash
cat sudocode-go/projects/migrations/1_initial.up.sql
```

```output
-- Specs table
CREATE TABLE specs (
    uuid         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   TEXT NOT NULL,
    id           TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    file_path    TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL DEFAULT '',
    priority     INT NOT NULL DEFAULT 2,
    archived     BOOLEAN NOT NULL DEFAULT false,
    archived_at  TIMESTAMPTZ,
    parent_id    TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, id)
);

CREATE INDEX idx_specs_project ON specs(project_id);

-- Issues table
CREATE TABLE issues (
    uuid         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   TEXT NOT NULL,
    id           TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'open',
    content      TEXT NOT NULL DEFAULT '',
    priority     INT NOT NULL DEFAULT 2,
    assignee     TEXT,
    archived     BOOLEAN NOT NULL DEFAULT false,
    archived_at  TIMESTAMPTZ,
    parent_id    TEXT,
    closed_at    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, id)
);

CREATE INDEX idx_issues_project ON issues(project_id);
CREATE INDEX idx_issues_status ON issues(project_id, status);

-- Tags table (shared for specs and issues)
CREATE TABLE tags (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id   TEXT NOT NULL,
    entity_type  TEXT NOT NULL, -- 'spec' or 'issue'
    entity_id    TEXT NOT NULL,
    tag          TEXT NOT NULL,
    UNIQUE (project_id, entity_type, entity_id, tag)
);

CREATE INDEX idx_tags_entity ON tags(project_id, entity_type, entity_id);

-- Relationships table
CREATE TABLE relationships (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id        TEXT NOT NULL,
    from_id           TEXT NOT NULL,
    from_type         TEXT NOT NULL,
    to_id             TEXT NOT NULL,
    to_type           TEXT NOT NULL,
    relationship_type TEXT NOT NULL,
    metadata          JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, from_id, to_id, relationship_type)
);

CREATE INDEX idx_relationships_from ON relationships(project_id, from_id);
CREATE INDEX idx_relationships_to ON relationships(project_id, to_id);

-- Feedback table
CREATE TABLE feedback (
    uuid          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    TEXT NOT NULL,
    id            TEXT NOT NULL,
    from_id       TEXT,
    to_id         TEXT NOT NULL,
    feedback_type TEXT NOT NULL DEFAULT 'comment',
    content       TEXT NOT NULL DEFAULT '',
    agent         TEXT,
    anchor        JSONB,
    dismissed     BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, id)
);

CREATE INDEX idx_feedback_to ON feedback(project_id, to_id);
```

Five tables, all multi-tenant via `project_id`. Key design choices:
- `specs` and `issues` use `UNIQUE (project_id, id)` so entity IDs are unique within a project
- `tags` are a separate EAV table rather than a JSON array — enables efficient tag-based queries
- `relationships` use a composite unique constraint preventing duplicate edges
- `feedback.anchor` is JSONB — stores the rich `FeedbackAnchor` struct for line-level comment positioning

```bash
grep "//encore:api" sudocode-go/projects/*.go
```

```output
sudocode-go/projects/feedback.go://encore:api auth method=POST path=/feedback/create
sudocode-go/projects/feedback.go://encore:api auth method=POST path=/feedback/update
sudocode-go/projects/feedback.go://encore:api auth method=POST path=/feedback/list
sudocode-go/projects/issues.go://encore:api auth method=POST path=/issues/list
sudocode-go/projects/issues.go://encore:api auth method=POST path=/issues/show
sudocode-go/projects/issues.go://encore:api auth method=POST path=/issues/upsert
sudocode-go/projects/issues.go://encore:api auth method=POST path=/issues/delete
sudocode-go/projects/relationships.go://encore:api auth method=POST path=/relationships/create
sudocode-go/projects/relationships.go://encore:api auth method=POST path=/relationships/delete
sudocode-go/projects/relationships.go://encore:api auth method=POST path=/relationships/list
sudocode-go/projects/specs.go://encore:api auth method=POST path=/specs/list
sudocode-go/projects/specs.go://encore:api auth method=POST path=/specs/show
sudocode-go/projects/specs.go://encore:api auth method=POST path=/specs/upsert
sudocode-go/projects/specs.go://encore:api auth method=POST path=/specs/delete
sudocode-go/projects/status.go://encore:api auth method=GET path=/ready
sudocode-go/projects/status.go://encore:api auth method=GET path=/status
```

16 endpoints, all requiring auth (`X-Project-ID`). The `Ready` endpoint is particularly important for agents — it returns issues that have no open blockers, which is how Sudocode's MCP tools surface "what to work on next".

```bash
sed -n "/func Ready/,/^}/p" sudocode-go/projects/status.go | head -40
```

```output
func Ready(ctx context.Context) (*ReadyResponse, error) {
	pid := projectID(ctx)

	rows, err := db.Query(ctx, `
		SELECT i.id, i.title, i.status, i.content, i.priority, i.assignee, i.archived, i.parent_id, i.closed_at, i.created_at, i.updated_at
		FROM issues i
		WHERE i.project_id = $1
		  AND i.status = 'open'
		  AND i.archived = false
		  AND NOT EXISTS (
		    SELECT 1 FROM relationships r
		    JOIN issues blocker ON blocker.project_id = $1 AND blocker.id = r.from_id AND blocker.status != 'closed'
		    WHERE r.project_id = $1
		      AND r.to_id = i.id
		      AND r.relationship_type = 'blocks'
		  )
		ORDER BY i.priority ASC, i.created_at ASC
	`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	issues, err := scanIssues(ctx, pid, rows)
	if err != nil {
		return nil, err
	}
	return &ReadyResponse{ReadyIssues: issues}, nil
}
```

The `NOT EXISTS` subquery is the key — it finds open issues where no other open issue blocks them via a `blocks` relationship. This is the "ready queue" that agents use to pick up work.

## 9. File Sync: `sync/`

The sync service bridges the database and the `.sudocode/` filesystem. On project open, it imports files into Postgres. On entity mutation, it exports back to files. This keeps the file-based workflow (git-friendly) in sync with the database (query-friendly).

```bash
cd sudocode-go && go test ./sync/ -v -run TestImportExport_RoundTrip -count=1 2>&1
```

```output
=== RUN   TestImportExport_RoundTrip_JSONL
--- PASS: TestImportExport_RoundTrip_JSONL (0.00s)
PASS
ok  	encore.app/sudocode-go/sync	0.189s
```

The round-trip test creates a `.sudocode/` directory in `t.TempDir()`, writes specs and issues, imports them, re-exports, and verifies all data survives. The sync service respects the `config.json` `sourceOfTruth` setting — when set to `markdown`, it reads from the markdown files; when `jsonl`, from the JSONL files. JSONL is always written regardless of mode.

## 10. WebSocket Realtime: `realtime/`

The realtime service provides a pub-sub hub for live event broadcasting. Clients connect via WebSocket and subscribe to entity changes filtered by project, entity type, and entity ID.

```bash
cd sudocode-go && go test ./realtime/ -v -run TestHubBroadcast -count=1 2>&1
```

```output
=== RUN   TestHubBroadcastToSubscribedClients
--- PASS: TestHubBroadcastToSubscribedClients (0.05s)
=== RUN   TestHubBroadcastEntityTypeFilter
--- PASS: TestHubBroadcastEntityTypeFilter (0.05s)
PASS
ok  	encore.app/sudocode-go/realtime	0.305s
```

The hub uses Go channels internally: a `register` channel for new clients, `unregister` for departing ones, and `broadcast` for events. Subscription matching supports wildcards — subscribing to just a `project_id` receives all events for that project, while subscribing with a specific `entity_type` and `entity_id` narrows the filter.

## 11. MCP Server: `mcpserver/`

The MCP (Model Context Protocol) server is how AI agents interact with Sudocode. It exposes 11 tools over HTTP/SSE at `/api/mcp` using the `mark3labs/mcp-go` SDK.

```bash
grep "mcp.NewTool(\"" sudocode-go/mcpserver/tools.go | sed "s/.*mcp.NewTool(\"//" | sed "s/\",.*//"
```

```output
ready
list_issues
show_issue
upsert_issue
list_specs
show_spec
upsert_spec
link
add_reference
add_feedback
get_project_id
```

Each tool handler extracts arguments from the MCP request, injects the `project_id` into the Encore auth context using `auth.WithContext`, calls the corresponding `projects` service endpoint, and returns JSON. The `link` and `add_reference` tools infer entity types (`spec`/`issue`) from the `s-`/`i-` ID prefix.

## 12. stdio MCP Shim: `cmd/sudocode-mcp/`

Many AI editors (Claude Code, Cursor) only support MCP over stdio (JSON-RPC on stdin/stdout). The shim binary bridges this to the HTTP daemon.

```bash
cd sudocode-go && go test ./cmd/sudocode-mcp/ -v -run TestProxyCall -count=1 2>&1
```

```output
=== RUN   TestProxyCall
--- PASS: TestProxyCall (0.00s)
=== RUN   TestProxyCallServerError
--- PASS: TestProxyCallServerError (0.00s)
PASS
ok  	encore.app/sudocode-go/cmd/sudocode-mcp	0.226s
```

The shim accepts `--project-id` and `--server-url` flags (or env vars `SUDOCODE_PROJECT_ID`, `SUDOCODE_SERVER_URL`). It registers all 11 tools as proxies — each tool handler POSTs the arguments as JSON to the daemon's HTTP API with the `X-Project-ID` header. The `TestProxyCall` test spins up a mock HTTP server to verify correct header passing and response forwarding.

## 13. Frontend Serving: `web/`

The web service embeds the pre-built TypeScript frontend SPA and serves it at `/web/*`.

```bash
cat sudocode-go/web/serve.go
```

```output
// Package web serves the pre-built frontend SPA as embedded static assets.
package web

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

//go:embed assets/*
var assets embed.FS

// assetsFS is the sub-filesystem rooted at "assets/".
var assetsFS, _ = fs.Sub(assets, "assets")

//encore:api public raw path=/web/*wildcard
func Serve(w http.ResponseWriter, req *http.Request) {
	// Strip the /web/ prefix to get the asset path.
	reqPath := strings.TrimPrefix(req.URL.Path, "/web/")
	if reqPath == "" || reqPath == "/" {
		reqPath = "index.html"
	}

	// Try to serve the requested file from embedded assets.
	if serveFile(w, reqPath) {
		return
	}

	// SPA fallback: serve index.html for non-asset paths.
	serveFile(w, "index.html")
}

// serveFile writes the named file from the embedded FS to w.
// Returns true if the file was found and served.
func serveFile(w http.ResponseWriter, name string) bool {
	f, err := assetsFS.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	// Check it's not a directory.
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return false
	}

	ext := filepath.Ext(name)
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f.(io.Reader))
	return true
}

// AssetFS exposes the embedded asset filesystem for testing.
func AssetFS() fs.FS {
	return assetsFS
}

// StripPrefix returns the asset path from a full request path.
func StripPrefix(requestPath string) string {
	p := strings.TrimPrefix(requestPath, "/web/")
	if p == "" {
		return "index.html"
	}
	return path.Clean(p)
}
```

`go:embed assets/*` bakes the frontend dist into the binary. The SPA fallback (line 32-33) ensures that deep links like `/web/project/i-6n7g` serve `index.html` so client-side routing can take over. In production, the TS frontend `dist/` is copied into `web/assets/` before `encore build docker`.

## 14. Full Test Suite

Running all tests that can execute without the Encore runtime (everything except `mcpserver` and `projects` which need `encore test` for Postgres):

```bash
cd sudocode-go && go test ./internal/... ./auth/... ./registry/... ./sync/... ./realtime/... ./web/... ./cmd/... -count=1 2>&1
```

```output
ok  	encore.app/sudocode-go/internal/jsonl	0.305s
ok  	encore.app/sudocode-go/internal/markdown	0.621s
ok  	encore.app/sudocode-go/internal/models	0.473s
ok  	encore.app/sudocode-go/internal/projectid	0.784s
ok  	encore.app/sudocode-go/auth	1.146s
ok  	encore.app/sudocode-go/registry	0.967s
ok  	encore.app/sudocode-go/sync	1.323s
ok  	encore.app/sudocode-go/realtime	1.576s
ok  	encore.app/sudocode-go/web	1.636s
ok  	encore.app/sudocode-go/cmd/sudocode-mcp	1.807s
```

All 10 packages pass. The remaining 2 (`mcpserver`, `projects`) require `encore test` which provisions a real Postgres database — they panic under plain `go test` because `sqldb.NewDatabase` needs the Encore runtime.

## Summary

| Layer | Package | What it does |
|-------|---------|-------------|
| Data models | `internal/models` | Go structs for spec, issue, relationship, feedback |
| ID generation | `internal/projectid` | Deterministic project IDs matching TypeScript |
| File I/O | `internal/jsonl`, `internal/markdown` | Read/write `.sudocode/` files |
| Auth | `auth` | Extract `X-Project-ID` from headers |
| Registry | `registry` | Project lifecycle, `projects.json` persistence |
| CRUD | `projects` | Postgres-backed spec/issue/relationship/feedback |
| Sync | `sync` | Bidirectional file ↔ DB synchronization |
| Realtime | `realtime` | WebSocket pub-sub for live updates |
| MCP | `mcpserver` | 11 AI agent tools over HTTP/SSE |
| Shim | `cmd/sudocode-mcp` | stdio ↔ HTTP bridge for editors |
| Frontend | `web` | Embedded SPA serving |

The codebase is ~7,100 lines of Go with ~2,900 lines of tests across 53 files, implementing a complete Sudocode server as a single Encore Go application.
