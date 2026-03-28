# Release Notes -- metafeather fork of sudocode

**Based on:** sudocode v0.2.0 (upstream commit `632de191`)
**Branch:** `01-15-customizations`
**Author:** Liam Clancy (metafeather)
**Date:** 2026-03-28

---

## Overview

This fork adds 37 commits (~4,800 lines) to upstream sudocode v0.2.0,
introducing agent flexibility, local-first workflow execution, automatic
project discovery from any nested directory, workflow UI improvements, and
general UX polish. All customizations are on the `01-15-customizations`
branch and are prefixed with `MF:` in commit messages.

---

## Features

### Agent Flexibility

- Default coding agent changed from `claude-code` to `opencode` in both the
  backend execution service and the frontend agent configuration panel.
- Added `opencode` and `gemini` as selectable agent options in the Create
  Workflow UI, broadening the set of supported orchestration targets.

### Project Discovery

- Allows using a sudocode project on a separate working directory, e.g. for use 
  with third party repositories.
- New offline-first project discovery module that resolves the correct
  sudocode project from any nested directory by reading the
  `~/.config/sudocode/projects.json` registry directly -- no running server
  required.
- Dynamic `SUDOCODE_DIR` resolution with a 5-level priority chain: explicit
  `--db` flag, `--working-dir` flag, `SUDOCODE_DIR` environment variable,
  project discovery, and walk-up search for `.sudocode/cache.db`.
- New `get_project_id` MCP tool and `config project-id` CLI command expose
  project resolution to external tools and agents.

### Workflow Engine Improvements

- Default workflow execution mode changed from `worktree` (git worktree
  isolation) to `local`, simplifying single-developer use.
- New `WorkflowExecutionMode` type (`"local" | "worktree"`) added to the
  shared type definitions. The `executionMode` configuration now propagates
  correctly through both the sequential and orchestrator workflow engines.
- Plan and Workflow resolution now performs recursive traversal of `blocks`
  dependency relationships, ensuring all blocking issues are included when
  building a spec source.
- Workflows no longer auto-start upon creation, giving users explicit
  control over when execution begins.

### Workflow UI Enhancements

- Issue status is now reflected as step status in the workflow view via a
  new `workflow-status.ts` utility that derives step state from the
  underlying issue state.
- Issue ID and title are displayed in workflow step headers for easier
  identification.
- Connected paths are highlighted in the workflow DAG view when hovering
  or selecting a step, making dependency chains easier to trace.
- Real-time workflow state updates via WebSocket subscription on the
  `useWorkflow()` hook, with corrected query keys for proper cache
  invalidation.
- `useIssues()` hook now defaults `archived` explicitly to `false`, and
  the server's `GET /api/issues` endpoint accepts `undefined` for
  unfiltered queries -- enabling workflow steps to display archived issues
  when needed.

### MCP & Server Fixes

- The execution service now injects `-w` (working directory) and `-d`
  (database path) flags into MCP server arguments, ensuring MCP tools
  operate on the correct project context.
- UI-initiated MCP executions follow the current project, while
  externally-initiated MCP executions respect the provided `workDir`.
- Fixed a bug where creating a Spec or Issue from the UI would use the
  wrong coding agent instead of the configured one.

### UI & UX Polish

- Added an Edit Spec button on the Spec detail page for inline editing.
- Default submit keybinding changed from Enter to Cmd+Return, reducing
  accidental submissions.
- Fixed a scroll-to-bottom jump that occurred when typing in chat and
  execution views.
- Increased UI timeout and added error handling for `SessionUpdate`
  failures to improve robustness on slower connections.
- Added iOS PWA support: `manifest.json`, maskable icon, and Apple-specific
  meta tags enable "Add to Home Screen" on iOS devices.

### Infrastructure & Testing

- Added `.envrc` for direnv integration (`source_up_if_exists` and
  `dotenv_if_exists`).
- Added `PRAGMA busy_timeout=5000` to the SQLite configuration to prevent
  `SQLITE_BUSY` errors under concurrent access.
- Fixed git commit operations when `.sudocode` is listed in `.gitignore`.
- Broad test stabilization pass: switched frontend tests to `localStorage`
  mocks, fixed CLI tests that depended on color output, fixed sync mixed
  state tests, and stabilized E2E tests. Approximately 2,800 new or
  modified test lines across the codebase.

---

## Compatibility

This fork rebases on upstream sudocode v0.2.0. All customizations are isolated
on the `01-15-customizations` branch. The `upstream` remote points to
`sudocode-ai/sudocode` for future rebasing.
