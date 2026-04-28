---
name: sudocode
description: ALWAYS use this skill for ALL sudocode spec and issue operations. Use when user mentions "spec", "issue", "ready", "blocked", "implement", "feature", "plan", or "feedback" with sudocode specs and issues. PROACTIVELY use at start of implementation tasks to check ready issues and understand work context. Operations include viewing (show_spec, show_issue, list_issues, list_specs), creating/modifying (upsert_spec, upsert_issue), planning features, breaking down work, creating dependency graphs, and providing implementation feedback.
---

# sudocode: Spec & Issue Management

Spec-driven development and issue management system. Work persists across sessions, specs guide implementation, dependency graphs ensure correct execution order, feedback loops close requirements gaps.

## Project Context (Required)

All sudocode operations are scoped to a **project**. Projects are registered in `~/.config/sudocode/projects.json` and identified by a deterministic `project_id`.

### MCP Tools

MCP tools (`ready`, `show_issue`, `upsert_spec`, etc.) handle project context automatically — the MCP client is started with a `--project-id` argument and sends it with every request. No extra steps needed when using MCP tools.

The sudocode **server** does not require a `project_id` to start — it receives `project_id` per-request via the `X-Project-ID` header from API/MCP clients, or from the UI when the user selects a project.

### CLI Commands

CLI commands **require `--project-id`** for all project-scoped operations. Without it, commands will fail.

```bash
# Get the project_id for the current directory
sudocode config project-id

# Pass it to any command
sudocode --project-id <id> issue list
sudocode --project-id <id> spec show s-xxxx
sudocode --project-id <id> ready
```

**Exempt commands** (work without `--project-id`): `init`, `server`, `config project-id`, `config get`, `config set`, `config show`, `merge-driver`, `init-merge-driver`, `remove-merge-driver`, `update`.

### Resolving project_id

- **MCP tool:** Use `get_project_id` tool (returns ID for current or specified path)
- **CLI:** Run `sudocode config project-id` in the project directory
- **Programmatic:** Read `~/.config/sudocode/projects.json` — each entry has `id` and `path`

The registry file is read directly from disk (offline-first) — no server needed for resolution.

### Important

- Never rely on `cwd`, env vars, or path ancestor matching to determine project context
- Always use an explicit `project_id` from the registry
- The `project_id` is stable even if the project directory is renamed/moved (it is updated in the registry)

## Core Concepts

- **Specs**: Requirements/design documents (markdown in `.sudocode/specs/`) - user-initiated, capture intent
- **Issues**: Work items with status tracking (markdown in `.sudocode/issues/`) - agent work, actionable tasks
- **Feedback**: Anchored comments on specs documenting what happened during implementation
- **Relationships**: Dependency graphs between issues and specs (blocks, implements, parent-child, discovered-from)

**Create issues when:** Concrete actionable work, can be completed and closed, implements a spec, is a bug/task
**Create specs when:** Documenting user intent and requirements, architecture decisions, API designs, feature specifications

## Working with the System

### Two Ways to Modify Specs/Issues

**Option 1: Direct Markdown Editing** (For content-heavy edits)

- Edit markdown files in `.sudocode/specs/` or `.sudocode/issues/`
- Frontmatter contains metadata (id, title, status, relationships, tags)
- Content after frontmatter is the body
- System syncs bidirectionally
- Use direct markdown editing when possible to maintain file structure and reduce content churn

**Option 2: MCP Tools** (Recommended for structured operations)

- Use `upsert_issue`, `upsert_spec`, `link`, `add_feedback` tools
- Automatically syncs to markdown/sqlite/jsonl
- Validates relationships and IDs

**When to use each:**

- **MCP tools:** Status changes, creating entities, adding relationships, adding feedback
- **Direct editing:** Writing detailed content, refactoring descriptions, bulk editing

### Obsidian-Style Mentions

**Link specs and issues inline using `[[ID]]` syntax:**

```markdown
Basic reference:
Implement OAuth per [[s-8h2k]]

With display text:
See [[s-8h2k|authentication spec]] for details

With relationship type:
Must complete [[i-7x9m]]{ blocks } first

Formats supported:

- [[s-14sh]] - basic reference (creates "references" relationship)
- [[i-x7k9]] or [[@i-x7k9]] - with @ prefix
- [[s-3s542|Custom Text]] - with display text
- [[s-x4d6df]]{ blocks } - declares relationship type
- [[s-24gfs3|Text]]{ blocks } - both display and relationship
```

**Relationship types in mentions:** `blocks`, `implements`, `depends-on`, `discovered-from`

**Why use inline mentions:**

- Bidirectionally links entities without separate `link` tool call
- Colocate with informational context
- Automatically creates relationships during sync
- Makes content more readable

## CLI Reference

When MCP tools are unavailable, use the `sudocode` CLI directly via terminal.

**All CLI commands require `--project-id`** (see Project Context section above). Examples below include it for clarity.

### Issue Commands

```bash
# Get your project_id first
PROJECT_ID=$(sudocode config project-id)

# Create — title is POSITIONAL, use -d for description (NOT --body)
sudocode --project-id $PROJECT_ID issue create '<title>' -p <0-4> -d '<description>' --tags '<comma,separated>' --parent <id>

# List
sudocode issue list                        # all issues
sudocode issue list --status open          # filter by status

# Show
sudocode issue show <id>

# Update — status can ONLY be set via update (not create)
sudocode issue update <id> -s <status>     # open|in_progress|blocked|needs_review|closed|wont_fix|duplicate
sudocode issue update <id> --title '<new>' --description '<new>'

# Close (shortcut for update --status closed)
sudocode issue close <id> -r '<reason>'
sudocode issue close <id1> <id2>           # close multiple

# Relationships
sudocode link <from-id> <to-id> -t <type>  # blocks|implements|depends-on|references|discovered-from
```

### Spec Commands

```bash
sudocode spec create '<title>' -p <0-4> -d '<description>' --tags '<tags>' --parent <id>
sudocode spec list
sudocode spec show <id>
sudocode spec update <id> --title '<new>' -d '<new>' --tags '<tags>'
```

### Other Commands

```bash
sudocode ready                             # Show unblocked issues
sudocode issue add-ref <entity-id> <ref-id>  # Add cross-reference
```

### CLI Gotchas

- **Title is positional:** `sudocode issue create 'My title'` not `--title 'My title'`
- **No --status on create:** Issues are created as `open`. Use `issue update <id> -s closed` after.
- **Description flag is -d:** Not `--body` or `--content`
- **Use single quotes** for shell arguments containing special characters (backticks, `!`, `$`, `"`)
- **Cross-references in description:** Use `[[i-xxxx]]` syntax — it creates relationships on sync

## Quick Reference

### Session Start (Always Do This)

```
- [ ] Resolve project_id (use get_project_id tool or `sudocode config project-id`)
- [ ] Use ready tool to find unblocked work
- [ ] Use list_issues with status=in_progress to see current work
- [ ] Ask user which work to pursue (if not specified)
```

If you were assigned an issue, work ONLY on implementing the requirements of the issue.

### Essential Tools (MCP)

```
get_project_id       → Resolve project_id for current directory
ready                → Find unblocked work
show_issue/show_spec → Get details with relationships
upsert_issue/spec    → Create/update (status, priority, parent)
link                 → Create relationships (blocks, implements, parent-child)
add_feedback         → Document implementation results on specs
```

### Relationship Types

| Type              | Purpose                   | Effect on ready                              |
| ----------------- | ------------------------- | -------------------------------------------- |
| `implements`      | Issue → Spec connection   | None (documentation)                         |
| `blocks`          | Execution ordering        | Blocked issue not ready until blocker closes |
| `parent-child`    | Hierarchical organization | None (hierarchy only)                        |
| `discovered-from` | Provenance tracking       | None (documentation)                         |

### Status Flow

Standard flow:

```
open → in_progress  → closed
  ↓         ↓            ↑
  └─────────┴────────────┘
    blocked (when waiting on dependencies)

Terminal states (no further transitions):
  any → wont_fix   (issue will not be addressed)
  any → duplicate  (duplicate of another issue)
```

When requirements are not fully met or unforeseen issues arise during execution:

```
in_progress → needs_review → closed
```

## When to Use Hierarchies

**Hierarchical specs:** Multiple subsystems, multiple layers, natural abstraction levels
**Hierarchical issues:** Epic with subtasks, clear dependencies, progress tracking at different granularity

### Pattern: Hierarchical Feature with Dependencies

**Example:** "Implement authentication system"

**Create hierarchy:**

```
s-2a7c: Auth System (parent)
├── s-8h2k: OAuth (child)
├── s-9j3m: Sessions (child)
└── s-4k8p: Permissions (child)

i-5n7q: Implement auth (parent epic, implements s-2a7c)
├── i-7x9m: OAuth flow (child, implements s-8h2k)
├── i-3p6k: Session storage (child, implements s-9j3m)
└── i-8w2n: Permissions (child, implements s-4k8p)
```

**Add execution order:**

```
link: i-7x9m blocks i-3p6k (OAuth before sessions)
link: i-3p6k blocks i-8w2n (sessions before permissions)
```

**Result:** `ready` shows i-7x9m → close it → `ready` shows i-3p6k → etc.

## Dependency Graphs

**Use `blocks` for:** Execution ordering (A must finish before B starts)
**Use `parent-child` for:** Hierarchical organization (tracking progress at multiple levels)
**Use both:** Parent-child for hierarchy + blocks for ordering

### Building Dependency Graphs

```
- [ ] Identify foundation work (must happen first)
- [ ] Identify parallel work (no dependencies)
- [ ] Create all issues first (don't worry about order)
- [ ] Add parent-child for hierarchy
- [ ] Add blocks for execution order
- [ ] Verify no circular dependencies
- [ ] Use ready to verify graph correct
```

**Pattern:** Foundation blocks everything else → parallel work in middle → validation at end

---

## Feedback Loop

**Always provide feedback when implementing specs.**

### When to Provide Feedback

- Complete implementing an issue that references a spec
- Discover requirements were unclear/incomplete
- Encounter implementation challenges not in spec
- Make design decisions that deviate from spec
- Have evidence requirements were met (tests, observations)

### Feedback Checklist

```
- [ ] Update issue status
- [ ] Use add_feedback on spec with requirements met, design decisions, challenges, evidence
- [ ] Choose type: comment (informational), suggestion (spec update), request (clarification)
- [ ] Anchor feedback to relevant spec sections
```

### Feedback Pattern Example

**Good feedback:**

```
✅ Requirements met: OAuth flow working per spec
📝 Design decisions: Used Redis for tokens (horizontal scaling), added rate limiting
⚠️ Challenges: PKCE needed for mobile (not in spec), token refresh race condition solved with 10s buffer
✅ Evidence: 47 tests passing, 3 OAuth providers tested, security scan clean
💡 Suggestions: Add mobile requirements, document token refresh edge case
```

**Bad feedback:** "Implemented the feature. It works."

---

## Status Transitions

### closed vs request_review

**Close (status=closed) when:**

- All requirements met
- Tests passing with good coverage
- Confident in approach
- No blocking questions

**Set needs_review when:**

- Uncertain about approach (multiple valid options)
- Partial completion (need priority guidance)
- Quality concerns
- Spec ambiguities found
- Trade-offs need approval

**Self-check:** "Could another developer deploy this to production as-is?"

- Yes confidently → Close
- Yes with caveats → Close + flag concerns in feedback
- Need guidance → Set needs_review + request clarification
- Incomplete → Set needs_review + explain gaps

### Closing Checklist

```
- [ ] All requirements met
- [ ] Tests passing
- [ ] No blocking questions
- [ ] Implementation sound or uncertainties flagged
- [ ] Evidence provided
- [ ] Feedback on spec documenting what was done
```

If any item is ✗, set needs_review with feedback explaining gaps/questions

---

## Common Workflows

**Spec-driven feature:** Create spec → Create issues that implement spec → Add blocks dependencies to establish execution order → Use ready to find next work → Provide feedback on spec when done

**Bug discovery:** Create issue for discovered work → Link with discovered-from → If blocker: add blocks relationship + set original to blocked

**Complex hierarchical feature:** Create parent spec + child specs → Create parent issue + child issues → Link issues to specs with implements → Add parent-child + blocks relationships → Use ready to execute in order → Provide feedback on each spec

---

## Integration with TodoWrite

**Use TodoWrite for:** Session-scoped checklists, immediate execution tracking
**Use Issues for:** Multi-session work, dependencies, spec implementation

**Pattern:** Start session with ready → Create TodoWrite for immediate tasks → Update issue status as you work → Close issue when complete
