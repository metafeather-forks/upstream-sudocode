# /// script
# requires-python = ">=3.11"
# dependencies = ["pytest"]
# ///
"""Tests for Phase 2: GitHub Issue creation/update with external_links tracking.

Acceptance criteria from i-1qgp:
- New entities get a GitHub Issue created with correct title, body, and labels
- external_links entry is written to JSONL only after confirmed GitHub Issue creation
- Re-running with unchanged content skips the entity (no API call)
- Re-running with changed content updates the existing GitHub Issue
- --force flag causes all entities to be updated regardless of content_hash
- Labels are auto-created in the repo if they don't exist
- JSONL writes are atomic (write to .tmp, then rename)
- Partial failures are reported (which entities succeeded vs failed)
"""

import hashlib
import json
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from unittest.mock import MagicMock, call, patch

import pytest

sys.path.insert(0, str(Path(__file__).parent.parent / "scripts"))
from export_to_github import (
    ExportSummary,
    GhResult,
    close_github_issue,
    compute_content_hash,
    find_external_link,
    ensure_labels,
    add_external_link,
    update_external_link,
    create_github_issue,
    update_github_issue,
    export_entity,
    export_entities,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _spec(
    id: str,
    title: str = "",
    parent_id: str | None = None,
    content: str = "",
    external_links: list | None = None,
    relationships: list | None = None,
) -> dict:
    return {
        "id": id,
        "uuid": f"uuid-{id}",
        "title": title or f"Spec {id}",
        "file_path": f"specs/{id}.md",
        "content": content,
        "priority": 1,
        "created_at": "2026-01-01 00:00:00",
        "updated_at": "2026-01-01 00:00:00",
        "parent_id": parent_id,
        "parent_uuid": f"uuid-{parent_id}" if parent_id else None,
        "relationships": relationships or [],
        "external_links": external_links or [],
        "tags": [],
    }


def _issue(
    id: str,
    title: str = "",
    parent_id: str | None = None,
    content: str = "",
    status: str = "open",
    external_links: list | None = None,
    relationships: list | None = None,
) -> dict:
    return {
        "id": id,
        "uuid": f"uuid-{id}",
        "title": title or f"Issue {id}",
        "content": content,
        "status": status,
        "priority": 1,
        "created_at": "2026-01-01 00:00:00",
        "updated_at": "2026-01-01 00:00:00",
        "parent_id": parent_id,
        "parent_uuid": f"uuid-{parent_id}" if parent_id else None,
        "relationships": relationships or [],
        "external_links": external_links or [],
        "tags": [],
    }


def _external_link(
    owner: str = "owner",
    repo: str = "repo",
    issue_number: int = 42,
    issue_id: int = 1234567890,
    content_hash: str = "abc123",
    entity_type: str = "spec",
) -> dict:
    return {
        "provider": "github",
        "external_id": f"{owner}/{repo}#{issue_number}",
        "external_url": f"https://github.com/{owner}/{repo}/issues/{issue_number}",
        "sync_enabled": True,
        "sync_direction": "outbound",
        "last_synced_at": "2026-01-01T00:00:00Z",
        "content_hash": content_hash,
        "metadata": {
            "github_issue_id": issue_id,
            "github_issue_number": issue_number,
            "owner": owner,
            "repo": repo,
            "entity_type": entity_type,
        },
    }


@pytest.fixture
def tmpdir():
    with tempfile.TemporaryDirectory() as d:
        yield Path(d)


# ---------------------------------------------------------------------------
# Test: find_external_link
# ---------------------------------------------------------------------------


class TestFindExternalLink:
    def test_no_external_links(self):
        """Entity with no external_links returns None."""
        entity = _spec("s-1234")
        result = find_external_link(entity, "owner", "repo")
        assert result is None

    def test_empty_external_links(self):
        """Entity with empty external_links array returns None."""
        entity = _spec("s-1234", external_links=[])
        result = find_external_link(entity, "owner", "repo")
        assert result is None

    def test_matching_link_found(self):
        """Returns the external_link matching provider/owner/repo."""
        link = _external_link(owner="owner", repo="repo")
        entity = _spec("s-1234", external_links=[link])
        result = find_external_link(entity, "owner", "repo")
        assert result is not None
        assert result["metadata"]["github_issue_number"] == 42

    def test_different_repo_not_matched(self):
        """Link for different repo is not returned."""
        link = _external_link(owner="owner", repo="other-repo")
        entity = _spec("s-1234", external_links=[link])
        result = find_external_link(entity, "owner", "repo")
        assert result is None

    def test_different_owner_not_matched(self):
        """Link for different owner is not returned."""
        link = _external_link(owner="other-owner", repo="repo")
        entity = _spec("s-1234", external_links=[link])
        result = find_external_link(entity, "other-owner", "other-repo")
        assert result is None

    def test_non_github_provider_ignored(self):
        """Non-github provider links are ignored."""
        link = _external_link(owner="owner", repo="repo")
        link["provider"] = "gitlab"
        entity = _spec("s-1234", external_links=[link])
        result = find_external_link(entity, "owner", "repo")
        assert result is None

    def test_multiple_links_correct_one_returned(self):
        """Multiple links, correct one is returned."""
        link1 = _external_link(owner="owner", repo="repo1", issue_number=10)
        link2 = _external_link(owner="owner", repo="repo2", issue_number=20)
        entity = _spec("s-1234", external_links=[link1, link2])
        result = find_external_link(entity, "owner", "repo2")
        assert result is not None
        assert result["metadata"]["github_issue_number"] == 20

    def test_null_external_links(self):
        """Entity with external_links=None returns None."""
        entity = _spec("s-1234")
        entity["external_links"] = None
        result = find_external_link(entity, "owner", "repo")
        assert result is None


# ---------------------------------------------------------------------------
# Test: ensure_labels
# ---------------------------------------------------------------------------


class TestEnsureLabels:
    @patch("export_to_github.run_gh")
    def test_label_exists_no_creation(self, mock_run_gh):
        """If label already exists, don't create it."""
        mock_run_gh.return_value = GhResult(
            success=True,
            stdout='[{"name":"spec"}]',
            stderr="",
            command=["gh", "label", "list"],
        )
        ensure_labels("owner/repo", "spec", dry_run=False)
        # Should only search, not create
        assert mock_run_gh.call_count == 1

    @patch("export_to_github.run_gh")
    def test_label_missing_creates_it(self, mock_run_gh):
        """If label doesn't exist, create it."""
        mock_run_gh.side_effect = [
            # Search returns empty
            GhResult(
                success=True,
                stdout="[]",
                stderr="",
                command=["gh", "label", "list"],
            ),
            # Create succeeds
            GhResult(
                success=True,
                stdout="",
                stderr="",
                command=["gh", "label", "create"],
            ),
        ]
        ensure_labels("owner/repo", "spec", dry_run=False)
        assert mock_run_gh.call_count == 2
        # Second call should be label create
        create_call = mock_run_gh.call_args_list[1]
        assert "label" in create_call[0][0]
        assert "create" in create_call[0][0]

    @patch("export_to_github.run_gh")
    def test_empty_label_skipped(self, mock_run_gh):
        """Empty label string should not trigger any gh calls."""
        ensure_labels("owner/repo", "", dry_run=False)
        mock_run_gh.assert_not_called()

    @patch("export_to_github.run_gh")
    def test_dry_run_passes_through(self, mock_run_gh):
        """Dry-run flag is passed to run_gh."""
        mock_run_gh.return_value = GhResult(
            success=True,
            stdout='[{"name":"spec"}]',
            stderr="",
            command=["gh", "label", "list"],
            dry_run=True,
        )
        ensure_labels("owner/repo", "spec", dry_run=True)
        # run_gh should be called with dry_run=True
        assert mock_run_gh.call_args[1].get("dry_run") is True


# ---------------------------------------------------------------------------
# Test: add_external_link / update_external_link (via sudocode CLI)
# ---------------------------------------------------------------------------


class TestAddExternalLink:
    @patch("export_to_github.run_sudocode")
    def test_adds_link(self, mock_run):
        """add_external_link invokes the CLI with correct args."""
        from export_to_github import SudocodeResult

        mock_run.return_value = SudocodeResult(
            success=True, stdout="", stderr="", command=[]
        )

        link = _external_link(owner="owner", repo="repo", content_hash="abc123")
        add_external_link("/tmp/.sudocode", "s-1234", link)

        mock_run.assert_called_once()
        cmd = mock_run.call_args[0][0]
        assert cmd[0] == "external-link"
        assert cmd[1] == "add"
        assert cmd[2] == "s-1234"
        assert "--provider" in cmd
        assert "--external-id" in cmd

    @patch("export_to_github.run_sudocode")
    def test_failure_raises(self, mock_run):
        """add_external_link raises RuntimeError on failure."""
        from export_to_github import SudocodeResult

        mock_run.return_value = SudocodeResult(
            success=False, stdout="", stderr="some error", command=[]
        )

        link = _external_link(owner="owner", repo="repo")
        with pytest.raises(RuntimeError, match="external-link add failed"):
            add_external_link("/tmp/.sudocode", "s-1234", link)


class TestUpdateExternalLink:
    @patch("export_to_github.run_sudocode")
    def test_updates_link(self, mock_run):
        """update_external_link invokes the CLI with correct args."""
        from export_to_github import SudocodeResult

        mock_run.return_value = SudocodeResult(
            success=True, stdout="", stderr="", command=[]
        )

        update_external_link(
            "/tmp/.sudocode",
            "s-1234",
            "owner/repo#42",
            content_hash="newhash",
            last_synced_at="2026-01-01T00:00:00Z",
        )

        mock_run.assert_called_once()
        cmd = mock_run.call_args[0][0]
        assert cmd[0] == "external-link"
        assert cmd[1] == "update"
        assert cmd[2] == "s-1234"
        assert "--content-hash" in cmd
        assert "--last-synced-at" in cmd

    @patch("export_to_github.run_sudocode")
    def test_failure_raises(self, mock_run):
        """update_external_link raises RuntimeError on failure."""
        from export_to_github import SudocodeResult

        mock_run.return_value = SudocodeResult(
            success=False, stdout="", stderr="update error", command=[]
        )

        with pytest.raises(RuntimeError, match="external-link update failed"):
            update_external_link(
                "/tmp/.sudocode", "s-1234", "owner/repo#42", content_hash="x"
            )


# ---------------------------------------------------------------------------
# Test: create_github_issue
# ---------------------------------------------------------------------------


class TestCreateGithubIssue:
    @patch("export_to_github.run_gh")
    def test_creates_issue_with_title_and_body(self, mock_run_gh):
        """Creates a GitHub Issue with correct title and body."""
        # gh issue create returns URL
        mock_run_gh.side_effect = [
            GhResult(
                success=True,
                stdout="https://github.com/owner/repo/issues/42\n",
                stderr="",
                command=["gh", "issue", "create"],
            ),
            # gh api to fetch issue ID
            GhResult(
                success=True,
                stdout="1234567890\n",
                stderr="",
                command=["gh", "api"],
            ),
        ]

        result = create_github_issue(
            repo="owner/repo",
            title="My Issue",
            body="Issue body",
            labels=["spec"],
            dry_run=False,
        )

        assert result is not None
        assert result["issue_number"] == 42
        assert result["issue_id"] == 1234567890
        assert result["url"] == "https://github.com/owner/repo/issues/42"

        # Verify the create call had the right args
        create_call = mock_run_gh.call_args_list[0]
        cmd = create_call[0][0]
        assert "--title" in cmd
        assert "My Issue" in cmd
        assert "--body" in cmd
        assert "Issue body" in cmd

    @patch("export_to_github.run_gh")
    def test_creates_issue_with_labels(self, mock_run_gh):
        """Labels are included in the create command."""
        mock_run_gh.side_effect = [
            GhResult(
                success=True,
                stdout="https://github.com/owner/repo/issues/42\n",
                stderr="",
                command=["gh", "issue", "create"],
            ),
            GhResult(
                success=True,
                stdout="1234567890\n",
                stderr="",
                command=["gh", "api"],
            ),
        ]

        create_github_issue(
            repo="owner/repo",
            title="Test",
            body="Body",
            labels=["spec", "feature"],
            dry_run=False,
        )

        create_cmd = mock_run_gh.call_args_list[0][0][0]
        # Check both labels are in the command
        label_indices = [i for i, arg in enumerate(create_cmd) if arg == "--label"]
        assert len(label_indices) == 2

    @patch("export_to_github.run_gh")
    def test_creates_issue_no_labels(self, mock_run_gh):
        """No labels when labels list is empty."""
        mock_run_gh.side_effect = [
            GhResult(
                success=True,
                stdout="https://github.com/owner/repo/issues/42\n",
                stderr="",
                command=["gh", "issue", "create"],
            ),
            GhResult(
                success=True,
                stdout="1234567890\n",
                stderr="",
                command=["gh", "api"],
            ),
        ]

        create_github_issue(
            repo="owner/repo",
            title="Test",
            body="Body",
            labels=[],
            dry_run=False,
        )

        create_cmd = mock_run_gh.call_args_list[0][0][0]
        assert "--label" not in create_cmd

    @patch("export_to_github.run_gh")
    def test_create_failure_returns_none(self, mock_run_gh):
        """If gh issue create fails, returns None."""
        mock_run_gh.return_value = GhResult(
            success=False,
            stdout="",
            stderr="Error creating issue",
            command=["gh", "issue", "create"],
        )

        result = create_github_issue(
            repo="owner/repo",
            title="Test",
            body="Body",
            labels=[],
            dry_run=False,
        )

        assert result is None

    @patch("export_to_github.run_gh")
    def test_dry_run_returns_placeholder(self, mock_run_gh):
        """Dry run returns a placeholder result."""
        mock_run_gh.return_value = GhResult(
            success=True,
            stdout="",
            stderr="",
            command=["gh", "issue", "create"],
            dry_run=True,
        )

        result = create_github_issue(
            repo="owner/repo",
            title="Test",
            body="Body",
            labels=["spec"],
            dry_run=True,
        )

        assert result is not None
        assert result["issue_number"] == 0
        assert result["issue_id"] == 0

    @patch("export_to_github.run_gh")
    def test_parses_issue_number_from_url(self, mock_run_gh):
        """Correctly parses issue number from the URL returned by gh."""
        mock_run_gh.side_effect = [
            GhResult(
                success=True,
                stdout="https://github.com/org/project/issues/123\n",
                stderr="",
                command=["gh", "issue", "create"],
            ),
            GhResult(
                success=True,
                stdout="9876543210\n",
                stderr="",
                command=["gh", "api"],
            ),
        ]

        result = create_github_issue(
            repo="org/project",
            title="Test",
            body="Body",
            labels=[],
            dry_run=False,
        )

        assert result["issue_number"] == 123
        assert result["issue_id"] == 9876543210

    @patch("export_to_github.time.sleep")
    @patch("export_to_github.run_gh")
    def test_id_fetch_retries_on_failure(self, mock_run_gh, mock_sleep):
        """ID fetch retries with backoff when first attempt fails."""
        mock_run_gh.side_effect = [
            # gh issue create succeeds
            GhResult(
                success=True,
                stdout="https://github.com/owner/repo/issues/42\n",
                stderr="",
                command=["gh", "issue", "create"],
            ),
            # First ID fetch fails
            GhResult(
                success=False,
                stdout="",
                stderr="HTTP 404",
                command=["gh", "api"],
            ),
            # Second ID fetch succeeds
            GhResult(
                success=True,
                stdout="1234567890\n",
                stderr="",
                command=["gh", "api"],
            ),
        ]

        result = create_github_issue(
            repo="owner/repo",
            title="Test",
            body="Body",
            labels=[],
            dry_run=False,
        )

        assert result is not None
        assert result["issue_number"] == 42
        assert result["issue_id"] == 1234567890
        # 3 run_gh calls: create + 2 ID fetch attempts
        assert mock_run_gh.call_count == 3

    @patch("export_to_github.time.sleep")
    @patch("export_to_github.run_gh")
    def test_id_fetch_all_retries_fail_returns_partial(self, mock_run_gh, mock_sleep):
        """When all ID fetch retries fail, returns partial result with issue_id=0."""
        mock_run_gh.side_effect = [
            # gh issue create succeeds
            GhResult(
                success=True,
                stdout="https://github.com/owner/repo/issues/42\n",
                stderr="",
                command=["gh", "issue", "create"],
            ),
            # All 3 ID fetch attempts fail
            GhResult(
                success=False, stdout="", stderr="HTTP 404", command=["gh", "api"]
            ),
            GhResult(
                success=False, stdout="", stderr="HTTP 404", command=["gh", "api"]
            ),
            GhResult(
                success=False, stdout="", stderr="HTTP 404", command=["gh", "api"]
            ),
        ]

        result = create_github_issue(
            repo="owner/repo",
            title="Test",
            body="Body",
            labels=[],
            dry_run=False,
        )

        assert result is not None
        assert result["issue_number"] == 42
        assert result["issue_id"] == 0  # sentinel value
        assert result["url"] == "https://github.com/owner/repo/issues/42"

    @patch("export_to_github.time.sleep")
    @patch("export_to_github.run_gh")
    def test_id_fetch_unparseable_returns_partial(self, mock_run_gh, mock_sleep):
        """When ID fetch returns non-integer, returns partial result with issue_id=0."""
        mock_run_gh.side_effect = [
            GhResult(
                success=True,
                stdout="https://github.com/owner/repo/issues/42\n",
                stderr="",
                command=["gh", "issue", "create"],
            ),
            GhResult(
                success=True,
                stdout="not-a-number\n",
                stderr="",
                command=["gh", "api"],
            ),
        ]

        result = create_github_issue(
            repo="owner/repo",
            title="Test",
            body="Body",
            labels=[],
            dry_run=False,
        )

        assert result is not None
        assert result["issue_number"] == 42
        assert result["issue_id"] == 0

    @patch("export_to_github.time.sleep")
    @patch("export_to_github.run_gh")
    def test_propagation_delay_after_creation(self, mock_run_gh, mock_sleep):
        """A 1-second delay is added after issue creation before ID fetch."""
        mock_run_gh.side_effect = [
            GhResult(
                success=True,
                stdout="https://github.com/owner/repo/issues/42\n",
                stderr="",
                command=["gh", "issue", "create"],
            ),
            GhResult(
                success=True,
                stdout="1234567890\n",
                stderr="",
                command=["gh", "api"],
            ),
        ]

        create_github_issue(
            repo="owner/repo",
            title="Test",
            body="Body",
            labels=[],
            dry_run=False,
        )

        # First sleep call should be the 1-second propagation delay
        assert mock_sleep.call_args_list[0] == call(1)


# ---------------------------------------------------------------------------
# Test: update_github_issue
# ---------------------------------------------------------------------------


class TestUpdateGithubIssue:
    @patch("export_to_github.run_gh")
    def test_updates_title_and_body(self, mock_run_gh):
        """Updates an existing GitHub Issue title and body."""
        mock_run_gh.return_value = GhResult(
            success=True,
            stdout="https://github.com/owner/repo/issues/42\n",
            stderr="",
            command=["gh", "issue", "edit"],
        )

        result = update_github_issue(
            repo="owner/repo",
            issue_number=42,
            title="Updated Title",
            body="Updated body",
            dry_run=False,
        )

        assert result is True
        cmd = mock_run_gh.call_args[0][0]
        assert "edit" in cmd
        assert "42" in cmd
        assert "--title" in cmd
        assert "Updated Title" in cmd
        assert "--body" in cmd
        assert "Updated body" in cmd

    @patch("export_to_github.run_gh")
    def test_update_failure_returns_false(self, mock_run_gh):
        """If gh issue edit fails, returns False."""
        mock_run_gh.return_value = GhResult(
            success=False,
            stdout="",
            stderr="Error editing issue",
            command=["gh", "issue", "edit"],
        )

        result = update_github_issue(
            repo="owner/repo",
            issue_number=42,
            title="Updated",
            body="Body",
            dry_run=False,
        )

        assert result is False

    @patch("export_to_github.run_gh")
    def test_dry_run_returns_true(self, mock_run_gh):
        """Dry run succeeds without making API calls."""
        mock_run_gh.return_value = GhResult(
            success=True,
            stdout="",
            stderr="",
            command=["gh", "issue", "edit"],
            dry_run=True,
        )

        result = update_github_issue(
            repo="owner/repo",
            issue_number=42,
            title="Updated",
            body="Body",
            dry_run=True,
        )

        assert result is True


# ---------------------------------------------------------------------------
# Test: export_entity - the create/update/skip orchestrator
# ---------------------------------------------------------------------------


class TestExportEntity:
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_new_entity_creates_issue(self, mock_create, mock_add, tmpdir):
        """Entity without external_link creates a new GitHub Issue."""
        spec = _spec("s-1234", title="My Spec", content="Spec content")

        mock_create.return_value = {
            "issue_number": 42,
            "issue_id": 1234567890,
            "url": "https://github.com/owner/repo/issues/42",
        }

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is not None
        assert result["issue_number"] == 42
        assert summary.created == 1
        mock_create.assert_called_once()
        mock_add.assert_called_once()

    @patch("export_to_github.update_external_link")
    @patch("export_to_github.update_github_issue")
    def test_changed_content_updates_issue(self, mock_update, mock_update_link, tmpdir):
        """Entity with changed content_hash updates the GitHub Issue."""
        old_hash = compute_content_hash("Old Title", "Old content")
        link = _external_link(
            owner="owner", repo="repo", issue_number=42, content_hash=old_hash
        )
        spec = _spec(
            "s-1234",
            title="New Title",
            content="New content",
            external_links=[link],
        )

        mock_update.return_value = True

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is not None
        assert summary.updated == 1
        mock_update.assert_called_once()
        mock_update_link.assert_called_once()

    @patch("export_to_github.update_external_link")
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.update_github_issue")
    @patch("export_to_github.create_github_issue")
    def test_unchanged_content_skips(
        self, mock_create, mock_update, mock_add, mock_update_link, tmpdir
    ):
        """Entity with unchanged content_hash is skipped."""
        title = "My Spec"
        content = "Spec content"
        current_hash = compute_content_hash(title, content)
        link = _external_link(
            owner="owner", repo="repo", issue_number=42, content_hash=current_hash
        )
        spec = _spec("s-1234", title=title, content=content, external_links=[link])

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is not None
        assert summary.skipped == 1
        mock_create.assert_not_called()
        mock_update.assert_not_called()
        mock_add.assert_not_called()
        mock_update_link.assert_not_called()

    @patch("export_to_github.update_external_link")
    @patch("export_to_github.update_github_issue")
    def test_force_updates_even_unchanged(self, mock_update, mock_update_link, tmpdir):
        """--force updates entity even when content_hash hasn't changed."""
        title = "My Spec"
        content = "Spec content"
        current_hash = compute_content_hash(title, content)
        link = _external_link(
            owner="owner", repo="repo", issue_number=42, content_hash=current_hash
        )
        spec = _spec("s-1234", title=title, content=content, external_links=[link])

        mock_update.return_value = True

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=True,
            summary=summary,
        )

        assert result is not None
        assert summary.updated == 1
        mock_update.assert_called_once()

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_create_failure_reported(self, mock_create, mock_add, tmpdir):
        """Failed creation increments failed count and doesn't write external link."""
        spec = _spec("s-1234", title="My Spec")

        mock_create.return_value = None  # failure

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is None
        assert summary.failed == 1
        mock_add.assert_not_called()

    @patch("export_to_github.update_external_link")
    @patch("export_to_github.update_github_issue")
    def test_update_failure_reported(self, mock_update, mock_update_link, tmpdir):
        """Failed update increments failed count."""
        old_hash = compute_content_hash("Old Title", "Old content")
        link = _external_link(
            owner="owner", repo="repo", issue_number=42, content_hash=old_hash
        )
        spec = _spec(
            "s-1234",
            title="New Title",
            content="New content",
            external_links=[link],
        )

        mock_update.return_value = False  # failure

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is None
        assert summary.failed == 1
        mock_update_link.assert_not_called()

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_spec_gets_spec_label(self, mock_create, mock_add, tmpdir):
        """Specs get the spec_label applied."""
        spec = _spec("s-1234")

        mock_create.return_value = {
            "issue_number": 42,
            "issue_id": 1234567890,
            "url": "https://github.com/owner/repo/issues/42",
        }

        summary = ExportSummary()
        export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        # Check that create was called with labels=["spec"]
        create_call = mock_create.call_args
        assert "spec" in create_call[1]["labels"]

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_issue_gets_issue_label(self, mock_create, mock_add, tmpdir):
        """Issues get the issue_label applied."""
        issue = _issue("i-5678")

        mock_create.return_value = {
            "issue_number": 43,
            "issue_id": 9876543210,
            "url": "https://github.com/owner/repo/issues/43",
        }

        summary = ExportSummary()
        export_entity(
            entity_id="i-5678",
            entity_type="issue",
            entity_data=issue,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="task",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        create_call = mock_create.call_args
        assert "task" in create_call[1]["labels"]

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_issue_empty_label_no_labels(self, mock_create, mock_add, tmpdir):
        """Issues with empty issue_label get no labels."""
        issue = _issue("i-5678")

        mock_create.return_value = {
            "issue_number": 43,
            "issue_id": 9876543210,
            "url": "https://github.com/owner/repo/issues/43",
        }

        summary = ExportSummary()
        export_entity(
            entity_id="i-5678",
            entity_type="issue",
            entity_data=issue,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        create_call = mock_create.call_args
        assert create_call[1]["labels"] == []

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_references_rewritten_in_body(self, mock_create, mock_add, tmpdir):
        """References in entity content are rewritten using the ref mapping."""
        spec = _spec("s-1234", title="My Spec", content="See [[s-5678]] for details")

        mock_create.return_value = {
            "issue_number": 42,
            "issue_id": 1234567890,
            "url": "https://github.com/owner/repo/issues/42",
        }

        summary = ExportSummary()
        export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={"s-5678": 10},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        create_call = mock_create.call_args
        assert "#10" in create_call[1]["body"]

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_partial_create_saves_external_link(self, mock_create, mock_add, tmpdir):
        """Fix 4: When issue_id=0 (ID fetch failed), external_link is still saved."""
        spec = _spec("s-1234", title="My Spec", content="Spec content")

        # Simulate partial result: issue created but ID fetch failed
        mock_create.return_value = {
            "issue_number": 42,
            "issue_id": 0,
            "url": "https://github.com/owner/repo/issues/42",
        }

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        # External link should be saved even with issue_id=0
        mock_add.assert_called_once()
        saved_link = mock_add.call_args[0][2]
        assert saved_link["metadata"]["github_issue_id"] == 0
        assert saved_link["metadata"]["github_issue_number"] == 42

        # Should be counted as failed (ID unknown)
        assert summary.failed == 1
        # Should return None since ID is unknown
        assert result is None

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_total_create_failure_no_external_link(self, mock_create, mock_add, tmpdir):
        """Fix 4: When create_github_issue returns None (total failure), no link is saved."""
        spec = _spec("s-1234", title="My Spec", content="Spec content")

        mock_create.return_value = None

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        # No link should be saved on total failure
        mock_add.assert_not_called()
        assert summary.failed == 1
        assert result is None

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_partial_create_dry_run_no_external_link(
        self, mock_create, mock_add, tmpdir
    ):
        """Fix 4: In dry run, external_link is not saved even for partial results."""
        spec = _spec("s-1234", title="My Spec", content="Spec content")

        mock_create.return_value = {
            "issue_number": 42,
            "issue_id": 0,
            "url": "https://github.com/owner/repo/issues/42",
        }

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=True,
            force=False,
            summary=summary,
        )

        # No link saved in dry run
        mock_add.assert_not_called()


# ---------------------------------------------------------------------------
# Test: search_github_issue - duplicate prevention
# ---------------------------------------------------------------------------


class TestSearchGithubIssue:
    """Tests for Fix 5: GitHub search fallback before creation."""

    @patch("export_to_github.run_gh")
    def test_exact_title_match_returns_number(self, mock_run_gh):
        """Exact title match returns the issue number."""
        from export_to_github import search_github_issue

        mock_run_gh.return_value = GhResult(
            success=True,
            stdout=json.dumps([{"number": 42, "title": "My Spec"}]),
            stderr="",
            command=["gh", "issue", "list"],
        )

        result = search_github_issue("owner/repo", "My Spec")

        assert result == 42
        cmd = mock_run_gh.call_args[0][0]
        assert "--search" in cmd

    @patch("export_to_github.run_gh")
    def test_no_match_returns_none(self, mock_run_gh):
        """No matching issues returns None."""
        from export_to_github import search_github_issue

        mock_run_gh.return_value = GhResult(
            success=True,
            stdout=json.dumps([]),
            stderr="",
            command=["gh", "issue", "list"],
        )

        result = search_github_issue("owner/repo", "My Spec")

        assert result is None

    @patch("export_to_github.run_gh")
    def test_fuzzy_match_ignored(self, mock_run_gh):
        """Non-exact title match is ignored."""
        from export_to_github import search_github_issue

        mock_run_gh.return_value = GhResult(
            success=True,
            stdout=json.dumps([{"number": 42, "title": "My Spec (updated)"}]),
            stderr="",
            command=["gh", "issue", "list"],
        )

        result = search_github_issue("owner/repo", "My Spec")

        assert result is None

    @patch("export_to_github.run_gh")
    def test_multiple_exact_matches_returns_none(self, mock_run_gh):
        """Multiple exact matches returns None (ambiguous)."""
        from export_to_github import search_github_issue

        mock_run_gh.return_value = GhResult(
            success=True,
            stdout=json.dumps(
                [
                    {"number": 42, "title": "My Spec"},
                    {"number": 43, "title": "My Spec"},
                ]
            ),
            stderr="",
            command=["gh", "issue", "list"],
        )

        result = search_github_issue("owner/repo", "My Spec")

        assert result is None

    @patch("export_to_github.run_gh")
    def test_api_failure_returns_none(self, mock_run_gh):
        """API failure returns None gracefully."""
        from export_to_github import search_github_issue

        mock_run_gh.return_value = GhResult(
            success=False,
            stdout="",
            stderr="some error",
            command=["gh", "issue", "list"],
        )

        result = search_github_issue("owner/repo", "My Spec")

        assert result is None

    @patch("export_to_github.run_gh")
    def test_invalid_json_returns_none(self, mock_run_gh):
        """Invalid JSON response returns None."""
        from export_to_github import search_github_issue

        mock_run_gh.return_value = GhResult(
            success=True,
            stdout="not json",
            stderr="",
            command=["gh", "issue", "list"],
        )

        result = search_github_issue("owner/repo", "My Spec")

        assert result is None

    @patch("export_to_github.run_gh")
    def test_dry_run_returns_none(self, mock_run_gh):
        """Dry run returns None (no search performed)."""
        from export_to_github import search_github_issue

        mock_run_gh.return_value = GhResult(
            success=True,
            stdout="",
            stderr="",
            command=["gh", "issue", "list"],
            dry_run=True,
        )

        result = search_github_issue("owner/repo", "My Spec", dry_run=True)

        assert result is None


class TestExportEntitySearchFallback:
    """Tests for Fix 5: search fallback in export_entity before creation."""

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.update_github_issue")
    @patch("export_to_github.search_github_issue")
    @patch("export_to_github.create_github_issue")
    def test_search_finds_existing_updates_instead(
        self, mock_create, mock_search, mock_update, mock_add, tmpdir
    ):
        """When search finds existing issue, update instead of create."""
        spec = _spec("s-1234", title="My Spec", content="Spec content")

        mock_search.return_value = 42
        mock_update.return_value = True

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        # Should NOT create, should update
        mock_create.assert_not_called()
        mock_update.assert_called_once()
        assert result is not None
        assert result["issue_number"] == 42
        assert summary.updated == 1

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.search_github_issue")
    @patch("export_to_github.create_github_issue")
    def test_search_finds_nothing_creates_normally(
        self, mock_create, mock_search, mock_add, tmpdir
    ):
        """When search finds nothing, proceed with normal creation."""
        spec = _spec("s-1234", title="My Spec", content="Spec content")

        mock_search.return_value = None
        mock_create.return_value = {
            "issue_number": 42,
            "issue_id": 1234567890,
            "url": "https://github.com/owner/repo/issues/42",
        }

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        mock_search.assert_called_once()
        mock_create.assert_called_once()
        assert result is not None
        assert summary.created == 1

    @patch("export_to_github.search_github_issue")
    @patch("export_to_github.create_github_issue")
    def test_search_not_called_when_link_exists(self, mock_create, mock_search, tmpdir):
        """Search is NOT called when an external_link already exists."""
        link = _external_link(
            owner="owner",
            repo="repo",
            issue_number=42,
            content_hash=compute_content_hash("My Spec", "Spec content"),
        )
        spec = _spec(
            "s-1234",
            title="My Spec",
            content="Spec content",
            external_links=[link],
        )

        summary = ExportSummary()
        export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        mock_search.assert_not_called()


# ---------------------------------------------------------------------------
# Test: export_entities - main loop
# ---------------------------------------------------------------------------


class TestExportEntities:
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    @patch("export_to_github.ensure_labels")
    def test_exports_all_entities(self, mock_ensure, mock_create, mock_add, tmpdir):
        """Exports all entities in order and builds ref_mapping."""
        spec = _spec("s-root", title="Root Spec", content="Root content")
        issue = _issue("i-impl", title="Implementation", content="Impl content")

        mock_create.side_effect = [
            {
                "issue_number": 1,
                "issue_id": 100,
                "url": "https://github.com/owner/repo/issues/1",
            },
            {
                "issue_number": 2,
                "issue_id": 200,
                "url": "https://github.com/owner/repo/issues/2",
            },
        ]

        sorted_entities = [
            ("s-root", "spec", spec),
            ("i-impl", "issue", issue),
        ]

        summary, ref_mapping, _id_mapping = export_entities(
            sorted_entities=sorted_entities,
            repo="owner/repo",
            project_id=tmpdir,
            spec_label="spec",
            issue_label="",
            dry_run=False,
            force=False,
        )

        assert summary.created == 2
        assert ref_mapping["s-root"] == 1
        assert ref_mapping["i-impl"] == 2

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    @patch("export_to_github.ensure_labels")
    def test_ref_mapping_used_for_later_entities(
        self, mock_ensure, mock_create, mock_add, tmpdir
    ):
        """Earlier entities' issue numbers are available in ref_mapping for later ones."""
        spec = _spec("s-root", title="Root")
        issue = _issue(
            "i-impl", title="Implementation", content="Implements [[s-root]]"
        )

        mock_create.side_effect = [
            {
                "issue_number": 10,
                "issue_id": 100,
                "url": "https://github.com/owner/repo/issues/10",
            },
            {
                "issue_number": 20,
                "issue_id": 200,
                "url": "https://github.com/owner/repo/issues/20",
            },
        ]

        sorted_entities = [
            ("s-root", "spec", spec),
            ("i-impl", "issue", issue),
        ]

        summary, ref_mapping, _id_mapping = export_entities(
            sorted_entities=sorted_entities,
            repo="owner/repo",
            project_id=tmpdir,
            spec_label="spec",
            issue_label="",
            dry_run=False,
            force=False,
        )

        # The second create call should have had s-root -> #10 available
        second_create_call = mock_create.call_args_list[1]
        body = second_create_call[1]["body"]
        assert "#10" in body

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    @patch("export_to_github.ensure_labels")
    def test_partial_failure_continues(
        self, mock_ensure, mock_create, mock_add, tmpdir
    ):
        """If one entity fails, other entities are still exported."""
        spec1 = _spec("s-aaa", title="First")
        spec2 = _spec("s-bbb", title="Second")

        mock_create.side_effect = [
            None,  # First fails
            {
                "issue_number": 2,
                "issue_id": 200,
                "url": "https://github.com/owner/repo/issues/2",
            },
        ]

        sorted_entities = [
            ("s-aaa", "spec", spec1),
            ("s-bbb", "spec", spec2),
        ]

        summary, ref_mapping, _id_mapping = export_entities(
            sorted_entities=sorted_entities,
            repo="owner/repo",
            project_id=tmpdir,
            spec_label="spec",
            issue_label="",
            dry_run=False,
            force=False,
        )

        assert summary.failed == 1
        assert summary.created == 1
        assert "s-bbb" in ref_mapping
        assert "s-aaa" not in ref_mapping

    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    @patch("export_to_github.ensure_labels")
    def test_ensure_labels_called(self, mock_ensure, mock_create, mock_add, tmpdir):
        """ensure_labels is called for both spec and issue labels."""
        spec = _spec("s-root")

        mock_create.return_value = {
            "issue_number": 1,
            "issue_id": 100,
            "url": "https://github.com/owner/repo/issues/1",
        }

        sorted_entities = [("s-root", "spec", spec)]

        export_entities(
            sorted_entities=sorted_entities,
            repo="owner/repo",
            project_id=tmpdir,
            spec_label="spec",
            issue_label="task",
            dry_run=False,
            force=False,
        )

        # ensure_labels should have been called for both labels
        calls = mock_ensure.call_args_list
        label_args = [c[0][1] for c in calls]
        assert "spec" in label_args
        assert "task" in label_args

    @patch("export_to_github.update_external_link")
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    @patch("export_to_github.ensure_labels")
    def test_existing_link_populates_ref_mapping(
        self, mock_ensure, mock_create, mock_add, mock_update_link, tmpdir
    ):
        """Entity with existing external_link has its number added to ref_mapping."""
        current_hash = compute_content_hash("Root", "content")
        link = _external_link(
            owner="owner",
            repo="repo",
            issue_number=99,
            content_hash=current_hash,
        )
        spec = _spec("s-root", title="Root", content="content", external_links=[link])

        sorted_entities = [("s-root", "spec", spec)]

        summary, ref_mapping, _id_mapping = export_entities(
            sorted_entities=sorted_entities,
            repo="owner/repo",
            project_id=tmpdir,
            spec_label="spec",
            issue_label="",
            dry_run=False,
            force=False,
        )

        assert ref_mapping["s-root"] == 99
        assert summary.skipped == 1

    @patch("export_to_github.time.sleep")
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    @patch("export_to_github.ensure_labels")
    def test_delay_between_operations(
        self, mock_ensure, mock_create, mock_add, mock_sleep, tmpdir
    ):
        """When delay > 0, time.sleep is called between entity exports."""
        spec = _spec("s-aaa", title="First")
        issue = _issue("i-bbb", title="Second")

        mock_create.side_effect = [
            {
                "issue_number": 1,
                "issue_id": 100,
                "url": "https://github.com/owner/repo/issues/1",
            },
            {
                "issue_number": 2,
                "issue_id": 200,
                "url": "https://github.com/owner/repo/issues/2",
            },
        ]

        sorted_entities = [
            ("s-aaa", "spec", spec),
            ("i-bbb", "issue", issue),
        ]

        export_entities(
            sorted_entities=sorted_entities,
            repo="owner/repo",
            project_id=tmpdir,
            spec_label="spec",
            issue_label="",
            dry_run=False,
            force=False,
            delay=0.5,
        )

        # time.sleep should be called with 0.5 for each entity
        delay_calls = [c for c in mock_sleep.call_args_list if c == call(0.5)]
        assert len(delay_calls) == 2

    @patch("export_to_github.time.sleep")
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    @patch("export_to_github.ensure_labels")
    def test_no_delay_in_dry_run(
        self, mock_ensure, mock_create, mock_add, mock_sleep, tmpdir
    ):
        """Delay is not applied in dry-run mode."""
        spec = _spec("s-aaa", title="First")

        mock_create.return_value = {
            "issue_number": 0,
            "issue_id": 0,
            "url": "",
        }

        sorted_entities = [("s-aaa", "spec", spec)]

        export_entities(
            sorted_entities=sorted_entities,
            repo="owner/repo",
            project_id=tmpdir,
            spec_label="spec",
            issue_label="",
            dry_run=True,
            force=False,
            delay=1.0,
        )

        # time.sleep should NOT be called with the delay value in dry-run
        delay_calls = [c for c in mock_sleep.call_args_list if c == call(1.0)]
        assert len(delay_calls) == 0


# ---------------------------------------------------------------------------
# Test: close_github_issue
# ---------------------------------------------------------------------------


class TestCloseGithubIssue:
    @patch("export_to_github.run_gh")
    def test_closes_issue_successfully(self, mock_run_gh):
        """Closing an open issue returns True."""
        mock_run_gh.return_value = GhResult(
            success=True, stdout="", stderr="", command=["gh", "issue", "close"]
        )

        result = close_github_issue(repo="owner/repo", issue_number=42)

        assert result is True
        cmd = mock_run_gh.call_args[0][0]
        assert "close" in cmd
        assert "42" in cmd
        assert "--repo" in cmd

    @patch("export_to_github.run_gh")
    def test_failure_returns_false(self, mock_run_gh):
        """If gh issue close fails, returns False."""
        mock_run_gh.return_value = GhResult(
            success=False, stdout="", stderr="Not found", command=[]
        )

        result = close_github_issue(repo="owner/repo", issue_number=42)

        assert result is False

    @patch("export_to_github.run_gh")
    def test_already_closed_returns_true(self, mock_run_gh):
        """If issue is already closed, returns True (idempotent)."""
        mock_run_gh.return_value = GhResult(
            success=False,
            stdout="",
            stderr="Issue is already closed",
            command=[],
        )

        result = close_github_issue(repo="owner/repo", issue_number=42)

        assert result is True

    @patch("export_to_github.run_gh")
    def test_dry_run_returns_true(self, mock_run_gh):
        """Dry-run mode returns True without making API calls."""
        mock_run_gh.return_value = GhResult(
            success=True, stdout="", stderr="", command=[], dry_run=True
        )

        result = close_github_issue(repo="owner/repo", issue_number=42, dry_run=True)

        assert result is True


# ---------------------------------------------------------------------------
# Test: export_entity - status sync
# ---------------------------------------------------------------------------


class TestExportEntityStatusSync:
    @patch("export_to_github.close_github_issue")
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_closed_issue_gets_closed_on_github(
        self, mock_create, mock_add, mock_close, tmpdir
    ):
        """Issue with status='closed' is closed on GitHub after creation."""
        issue = _issue("i-done", title="Done Task", status="closed")

        mock_create.return_value = {
            "issue_number": 42,
            "issue_id": 1234567890,
            "url": "https://github.com/owner/repo/issues/42",
        }
        mock_close.return_value = True

        summary = ExportSummary()
        result = export_entity(
            entity_id="i-done",
            entity_type="issue",
            entity_data=issue,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="",
            issue_label="task",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is not None
        assert summary.created == 1
        mock_close.assert_called_once_with(
            repo="owner/repo", issue_number=42, dry_run=False
        )

    @patch("export_to_github.close_github_issue")
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_open_issue_not_closed(self, mock_create, mock_add, mock_close, tmpdir):
        """Issue with status='open' is NOT closed on GitHub."""
        issue = _issue("i-open", title="Open Task", status="open")

        mock_create.return_value = {
            "issue_number": 43,
            "issue_id": 9876543210,
            "url": "https://github.com/owner/repo/issues/43",
        }

        summary = ExportSummary()
        result = export_entity(
            entity_id="i-open",
            entity_type="issue",
            entity_data=issue,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="",
            issue_label="task",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is not None
        assert summary.created == 1
        mock_close.assert_not_called()

    @patch("export_to_github.close_github_issue")
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_close_failure_returns_none(
        self, mock_create, mock_add, mock_close, tmpdir
    ):
        """If closing the issue fails, export_entity returns None and increments failed."""
        issue = _issue("i-done", title="Done Task", status="closed")

        mock_create.return_value = {
            "issue_number": 42,
            "issue_id": 1234567890,
            "url": "https://github.com/owner/repo/issues/42",
        }
        mock_close.return_value = False  # Close fails

        summary = ExportSummary()
        result = export_entity(
            entity_id="i-done",
            entity_type="issue",
            entity_data=issue,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="",
            issue_label="task",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is None
        # Created + failed (close failure adds 1 to failed)
        assert summary.created == 1
        assert summary.failed == 1

    @patch("export_to_github.close_github_issue")
    @patch("export_to_github.update_external_link")
    @patch("export_to_github.update_github_issue")
    def test_closed_issue_gets_closed_on_update(
        self, mock_update, mock_update_link, mock_close, tmpdir
    ):
        """Issue with status='closed' is also closed when updating existing issue."""
        old_hash = compute_content_hash("Old Title", "Old content")
        link = _external_link(
            owner="owner",
            repo="repo",
            issue_number=42,
            content_hash=old_hash,
            entity_type="issue",
        )
        issue = _issue(
            "i-done",
            title="New Title",
            content="New content",
            status="closed",
            external_links=[link],
        )

        mock_update.return_value = True
        mock_close.return_value = True

        summary = ExportSummary()
        result = export_entity(
            entity_id="i-done",
            entity_type="issue",
            entity_data=issue,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="",
            issue_label="task",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is not None
        assert summary.updated == 1
        mock_close.assert_called_once()

    @patch("export_to_github.close_github_issue")
    @patch("export_to_github.update_external_link")
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.update_github_issue")
    @patch("export_to_github.create_github_issue")
    def test_closed_issue_skipped_still_closes(
        self, mock_create, mock_update, mock_add, mock_update_link, mock_close, tmpdir
    ):
        """Skipped entity with status='closed' still gets closed on GitHub."""
        title = "Done Spec"
        content = "Done content"
        current_hash = compute_content_hash(title, content)
        link = _external_link(
            owner="owner",
            repo="repo",
            issue_number=42,
            content_hash=current_hash,
        )
        issue = _issue(
            "i-done",
            title=title,
            content=content,
            status="closed",
            external_links=[link],
        )

        mock_close.return_value = True

        summary = ExportSummary()
        result = export_entity(
            entity_id="i-done",
            entity_type="issue",
            entity_data=issue,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="",
            issue_label="task",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is not None
        assert summary.skipped == 1
        # Even though content was skipped, the close should still happen
        mock_close.assert_called_once()
        mock_create.assert_not_called()
        mock_update.assert_not_called()

    @patch("export_to_github.close_github_issue")
    @patch("export_to_github.add_external_link")
    @patch("export_to_github.create_github_issue")
    def test_entity_without_status_defaults_to_open(
        self, mock_create, mock_add, mock_close, tmpdir
    ):
        """Entity without a status field defaults to 'open' (no close)."""
        spec = _spec("s-1234", title="My Spec")
        # specs don't have status field

        mock_create.return_value = {
            "issue_number": 42,
            "issue_id": 1234567890,
            "url": "https://github.com/owner/repo/issues/42",
        }

        summary = ExportSummary()
        result = export_entity(
            entity_id="s-1234",
            entity_type="spec",
            entity_data=spec,
            repo="owner/repo",
            owner="owner",
            repo_name="repo",
            ref_mapping={},
            spec_label="spec",
            issue_label="",
            project_id=tmpdir,
            dry_run=False,
            force=False,
            summary=summary,
        )

        assert result is not None
        mock_close.assert_not_called()
