---
name: sudocode-export-github
description: >
  Export Sudocode specs and issues to GitHub Issues. Triggers on: "export to github",
  "sync to github", "push to github", "github export", "create github issues from spec",
  or when user wants to share sudocode work as GitHub Issues.
---

# sudocode-export-github: Export to GitHub Issues

Export a Sudocode spec and its full dependency graph (child specs, implementing issues, relationships, feedback) to GitHub Issues. Tracks mappings locally via `external_links` for incremental sync.

## Prerequisites

- `gh` CLI installed and authenticated (`gh auth status`)
- Target GitHub repository exists and is accessible
- `uv` installed (runs the script with inline metadata, no venv needed)
- A sudocode project with a known `project_id`

## Usage

### Determine the project ID

Resolve the `project_id` from the project registry:

1. Use `get_project_id` MCP tool, or run `sudocode config project-id` in the project directory
2. The project ID is stable and portable — no need to know the `.sudocode/` directory path

### Run the export

```bash
uv run ~/.agents/skills/sudocode/sudocode-export-github/scripts/export_to_github.py \
  --spec-id <SPEC_ID> \
  --repo <OWNER/REPO> \
  --project-id <PROJECT_ID> \
  [--spec-label spec] \
  [--issue-label ""] \
  [--dry-run] \
  [--force]
```

### Options

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--spec-id` | Yes | — | Root spec ID to export (e.g. `s-2a7c`) |
| `--repo` | Yes | — | Target GitHub repo in `owner/repo` format |
| `--project-id` | Yes | — | Sudocode project ID (from `sudocode config project-id`) |
| `--spec-label` | No | `spec` | Label applied to spec GitHub Issues |
| `--issue-label` | No | (none) | Label applied to issue GitHub Issues |
| `--dry-run` | No | false | Print planned actions without making API calls or modifying JSONL |
| `--force` | No | false | Re-export all entities regardless of content hash |

### Recommended workflow

1. **Dry-run first** to verify the export plan:
   ```bash
   uv run ~/.agents/skills/sudocode/sudocode-export-github/scripts/export_to_github.py \
     --spec-id s-2a7c --repo owner/repo --project-id my-proj-abc123 --dry-run
   ```
2. **Run the actual export:**
   ```bash
   uv run ~/.agents/skills/sudocode/sudocode-export-github/scripts/export_to_github.py \
     --spec-id s-2a7c --repo owner/repo --project-id my-proj-abc123
   ```
3. **Re-run for incremental updates** — only changed entities are updated, unchanged ones are skipped.

### Post-export

After a successful export, display the summary to the user. The root spec's GitHub Issue URL can be found in its `external_links` entry in `specs.jsonl`, or from the export output.

## What gets exported

### Entities
- The root spec and all transitive child specs
- All issues that implement any collected spec
- Entities are created in topological order (parents before children, blockers before blocked)

### Relationships
| Sudocode | GitHub Feature | Notes |
|----------|---------------|-------|
| Parent-child (spec hierarchy) | Sub-Issues | Parent spec becomes parent issue |
| `implements` (issue -> spec) | Sub-Issues | Issue becomes sub-issue of spec's GH issue |
| `blocks` / `depends-on` | Issue Dependencies | Uses numeric issue ID, not number |
| `references` / `related` / `discovered-from` | Comment mention | `Related: #N` comment |

### Content
- `[[s-XXXX]]` references in descriptions are rewritten to `#N` GitHub links
- `[[s-XXXX|Display Text]]` becomes `Display Text (#N)`
- Content hash (SHA-256 of title+description) enables incremental sync
- Labels are created in the repo if they don't exist

### Feedback
- Feedback entries targeting exported specs are posted as comments on the spec's GitHub Issue
- Comment headers use `**[Feedback from #N]**` (originating issue's GitHub Issue number) for navigability, falling back to feedback type when the originating issue wasn't exported
- Feedback comments are posted in chronological order (sorted by `created_at`)
- No footer — comments contain only header, anchor context, and content
- Deduplication via content hash prevents re-posting on subsequent runs

### Status Sync
- Closed Sudocode issues are automatically closed on GitHub after creation/update
- Specs (which have no status field) default to open
- Already-closed GitHub Issues are handled idempotently

## Tracking

Each exported entity gets an `external_links` entry in the JSONL file with:
- `provider: "github"`
- `sync_direction: "outbound"`
- `content_hash` for change detection
- `metadata.github_issue_id` (numeric ID for API calls)
- `metadata.github_issue_number` (for display)

## Error handling

- Fails gracefully if `gh` is not authenticated or repo doesn't exist
- **Hard failure on any errors**: exits with non-zero status and prints a breakdown of entity/relationship/feedback failures. Failures leave GitHub issues out of sync and must not be silently tolerated.
- `create_github_issue()` returns `None` (not `0`) when the numeric ID fetch fails
- Guard validation rejects ID=0/number=0 in relationship establishment
- Rate limiting: exponential backoff on HTTP 429
- All relationship APIs are idempotent (safe to re-run)

## Reference

See `references/relationship-mapping.md` for detailed GitHub API mapping, endpoint details, and troubleshooting.
