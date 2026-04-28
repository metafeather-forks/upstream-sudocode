"""Tests for Phase 4: Feedback export as GitHub Issue comments.

Tests cover:
- Feedback collection from entities in the export set
- Feedback content hash computation (for deduplication)
- Feedback comment formatting (type, anchor context, agent)
- Creating comments via gh CLI
- Deduplication: already-exported feedback is skipped
- Incremental export: new feedback only
- JSONL external_link metadata.exported_feedback[] tracking
- Integration with export pipeline
"""

from __future__ import annotations

import hashlib
import json
import sys
import textwrap
from pathlib import Path
from unittest.mock import MagicMock, call, patch

import pytest

sys.path.insert(
    0,
    str(Path(__file__).resolve().parent.parent / "scripts"),
)

from export_to_github import (
    GhResult,
    collect_feedback_for_export,
    compute_feedback_hash,
    export_feedback,
    FeedbackSummary,
    format_feedback_comment,
    post_feedback_comment,
    update_external_link,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_issue(
    issue_id: str,
    feedback: list[dict] | None = None,
    external_links: list[dict] | None = None,
) -> dict:
    """Create a minimal issue dict for testing."""
    d = {
        "id": issue_id,
        "title": f"Issue {issue_id}",
        "content": "",
        "relationships": [],
    }
    if feedback is not None:
        d["feedback"] = feedback
    if external_links is not None:
        d["external_links"] = external_links
    return d


def _make_spec(
    spec_id: str,
    external_links: list[dict] | None = None,
) -> dict:
    """Create a minimal spec dict for testing."""
    d = {
        "id": spec_id,
        "title": f"Spec {spec_id}",
        "content": "",
    }
    if external_links is not None:
        d["external_links"] = external_links
    return d


def _make_feedback(
    fb_id: str = "fb-001",
    from_id: str = "i-abc",
    to_id: str = "s-xyz",
    feedback_type: str = "comment",
    content: str = "Some feedback",
    agent: str = "test-agent",
    anchor: str | None = None,
) -> dict:
    return {
        "id": fb_id,
        "from_id": from_id,
        "to_id": to_id,
        "feedback_type": feedback_type,
        "content": content,
        "agent": agent,
        "anchor": anchor,
        "dismissed": False,
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    }


def _make_ext_link(
    owner: str = "owner",
    repo: str = "repo",
    issue_number: int = 42,
    exported_feedback: list[str] | None = None,
) -> dict:
    link = {
        "provider": "github",
        "external_id": f"{owner}/{repo}#{issue_number}",
        "external_url": f"https://github.com/{owner}/{repo}/issues/{issue_number}",
        "sync_enabled": True,
        "sync_direction": "outbound",
        "last_synced_at": "2026-01-01T00:00:00Z",
        "content_hash": "abc123",
        "metadata": {
            "github_issue_id": 1000 + issue_number,
            "github_issue_number": issue_number,
            "owner": owner,
            "repo": repo,
            "entity_type": "spec",
        },
    }
    if exported_feedback is not None:
        link["metadata"]["exported_feedback"] = exported_feedback
    return link


# ---------------------------------------------------------------------------
# compute_feedback_hash
# ---------------------------------------------------------------------------


class TestComputeFeedbackHash:
    def test_basic_hash(self):
        result = compute_feedback_hash("hello world")
        expected = hashlib.sha256("hello world".encode("utf-8")).hexdigest()
        assert result == expected

    def test_deterministic(self):
        assert compute_feedback_hash("test") == compute_feedback_hash("test")

    def test_different_content_different_hash(self):
        assert compute_feedback_hash("a") != compute_feedback_hash("b")

    def test_empty_string(self):
        result = compute_feedback_hash("")
        expected = hashlib.sha256(b"").hexdigest()
        assert result == expected


# ---------------------------------------------------------------------------
# collect_feedback_for_export
# ---------------------------------------------------------------------------


class TestCollectFeedbackForExport:
    def test_collects_feedback_targeting_spec_in_export_set(self):
        """Feedback from an issue that targets a spec in our export set is collected."""
        fb = _make_feedback(to_id="s-xyz", from_id="i-abc")
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 1
        assert result[0]["to_id"] == "s-xyz"

    def test_ignores_feedback_targeting_spec_not_in_export_set(self):
        """Feedback targeting a spec not in the export set is excluded."""
        fb = _make_feedback(to_id="s-other", from_id="i-abc")
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 0

    def test_collects_multiple_feedback_entries(self):
        fb1 = _make_feedback(fb_id="fb-1", to_id="s-xyz", content="First")
        fb2 = _make_feedback(fb_id="fb-2", to_id="s-xyz", content="Second")
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb1, fb2])),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 2

    def test_collects_from_multiple_issues(self):
        fb1 = _make_feedback(fb_id="fb-1", from_id="i-1", to_id="s-xyz")
        fb2 = _make_feedback(fb_id="fb-2", from_id="i-2", to_id="s-xyz")
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("i-1", "issue", _make_issue("i-1", feedback=[fb1])),
            ("i-2", "issue", _make_issue("i-2", feedback=[fb2])),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 2

    def test_no_feedback_field(self):
        """Issues without a feedback field are handled gracefully."""
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("i-abc", "issue", _make_issue("i-abc")),  # no feedback field
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 0

    def test_empty_feedback_array(self):
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[])),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 0

    def test_feedback_on_specs_also_collected(self):
        """Feedback stored directly on a spec entity is also collected."""
        fb = _make_feedback(to_id="s-xyz")
        spec = _make_spec("s-xyz")
        spec["feedback"] = [fb]
        entities = [
            ("s-xyz", "spec", spec),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 1

    def test_dismissed_feedback_excluded(self):
        """Dismissed feedback entries are excluded."""
        fb = _make_feedback(to_id="s-xyz")
        fb["dismissed"] = True
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 0

    def test_feedback_targeting_multiple_specs(self):
        """Feedback entries targeting different specs are all collected."""
        fb1 = _make_feedback(fb_id="fb-1", to_id="s-xyz")
        fb2 = _make_feedback(fb_id="fb-2", to_id="s-abc")
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("s-abc", "spec", _make_spec("s-abc")),
            ("i-1", "issue", _make_issue("i-1", feedback=[fb1, fb2])),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 2

    def test_feedback_sorted_by_created_at(self):
        """Feedback entries are returned sorted by created_at ascending."""
        fb_late = _make_feedback(fb_id="fb-late", to_id="s-xyz", content="Late")
        fb_late["created_at"] = "2026-04-12T20:00:00Z"
        fb_early = _make_feedback(fb_id="fb-early", to_id="s-xyz", content="Early")
        fb_early["created_at"] = "2026-04-12T10:00:00Z"
        fb_mid = _make_feedback(fb_id="fb-mid", to_id="s-xyz", content="Mid")
        fb_mid["created_at"] = "2026-04-12T15:00:00Z"
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            # Provide in reverse order to ensure sorting happens
            ("i-1", "issue", _make_issue("i-1", feedback=[fb_late, fb_mid, fb_early])),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 3
        assert result[0]["content"] == "Early"
        assert result[1]["content"] == "Mid"
        assert result[2]["content"] == "Late"

    def test_feedback_sorted_across_multiple_issues(self):
        """Feedback from different issues is interleaved by created_at."""
        fb1 = _make_feedback(
            fb_id="fb-1", from_id="i-1", to_id="s-xyz", content="First"
        )
        fb1["created_at"] = "2026-04-12T10:00:00Z"
        fb2 = _make_feedback(
            fb_id="fb-2", from_id="i-2", to_id="s-xyz", content="Third"
        )
        fb2["created_at"] = "2026-04-12T20:00:00Z"
        fb3 = _make_feedback(
            fb_id="fb-3", from_id="i-1", to_id="s-xyz", content="Second"
        )
        fb3["created_at"] = "2026-04-12T15:00:00Z"
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("i-1", "issue", _make_issue("i-1", feedback=[fb1, fb3])),
            ("i-2", "issue", _make_issue("i-2", feedback=[fb2])),
        ]
        result = collect_feedback_for_export(entities)
        assert len(result) == 3
        assert result[0]["content"] == "First"
        assert result[1]["content"] == "Second"
        assert result[2]["content"] == "Third"


# ---------------------------------------------------------------------------
# format_feedback_comment
# ---------------------------------------------------------------------------


class TestFormatFeedbackComment:
    def test_basic_comment_format(self):
        fb = _make_feedback(
            feedback_type="comment",
            content="Implementation done",
            agent="agent-1",
        )
        result = format_feedback_comment(fb)
        assert "**[comment]**" in result
        assert "Implementation done" in result
        # Footer and agent should NOT be present
        assert "agent-1" not in result
        assert "Exported from Sudocode" not in result

    def test_suggestion_type(self):
        fb = _make_feedback(feedback_type="suggestion", content="Try this")
        result = format_feedback_comment(fb)
        assert "**[suggestion]**" in result
        assert "Try this" in result

    def test_request_type(self):
        fb = _make_feedback(feedback_type="request", content="Clarify X")
        result = format_feedback_comment(fb)
        assert "**[request]**" in result

    def test_anchor_with_line_number(self):
        anchor = json.dumps({"line_number": 42, "text_snippet": "some code"})
        fb = _make_feedback(content="Fix this", anchor=anchor)
        result = format_feedback_comment(fb)
        assert "Line 42" in result
        assert "some code" in result

    def test_anchor_with_section_heading(self):
        anchor = json.dumps(
            {
                "section_heading": "Phase 2: Create Issues",
                "line_number": 79,
            }
        )
        fb = _make_feedback(content="Looks good", anchor=anchor)
        result = format_feedback_comment(fb)
        assert "Phase 2: Create Issues" in result

    def test_anchor_none(self):
        fb = _make_feedback(content="General feedback", anchor=None)
        result = format_feedback_comment(fb)
        assert "General feedback" in result
        # Should not crash or include "None"
        assert "None" not in result

    def test_anchor_invalid_json(self):
        fb = _make_feedback(content="Feedback", anchor="not-json{")
        result = format_feedback_comment(fb)
        # Should not crash, still include content
        assert "Feedback" in result

    def test_no_footer(self):
        """Footer should not be present in formatted comment."""
        fb = _make_feedback(agent="my-agent")
        result = format_feedback_comment(fb)
        assert "Exported from Sudocode feedback" not in result
        assert "my-agent" not in result
        assert "---" not in result

    def test_header_uses_github_issue_number(self):
        """When ref_mapping contains from_id, header uses 'Feedback from #N'."""
        fb = _make_feedback(from_id="i-abc", feedback_type="comment")
        ref_mapping = {"i-abc": 99}
        result = format_feedback_comment(fb, ref_mapping=ref_mapping)
        assert "**[Feedback from #99]**" in result
        assert "**[comment]**" not in result

    def test_header_falls_back_to_type_without_ref_mapping(self):
        """Without ref_mapping, header falls back to feedback_type."""
        fb = _make_feedback(feedback_type="suggestion")
        result = format_feedback_comment(fb)
        assert "**[suggestion]**" in result

    def test_header_falls_back_when_from_id_not_in_mapping(self):
        """When from_id is not in ref_mapping, header falls back to feedback_type."""
        fb = _make_feedback(from_id="i-unknown", feedback_type="request")
        ref_mapping = {"i-other": 42}
        result = format_feedback_comment(fb, ref_mapping=ref_mapping)
        assert "**[request]**" in result
        assert "#42" not in result


# ---------------------------------------------------------------------------
# post_feedback_comment
# ---------------------------------------------------------------------------


class TestPostFeedbackComment:
    @patch("export_to_github.run_gh")
    def test_posts_comment_via_gh(self, mock_run_gh):
        mock_run_gh.return_value = GhResult(
            success=True, stdout="", stderr="", command=[]
        )
        result = post_feedback_comment(
            issue_number=42,
            body="test comment",
            repo="owner/repo",
        )
        assert result is True
        mock_run_gh.assert_called_once()
        cmd = mock_run_gh.call_args[0][0]
        assert "gh" in cmd
        assert "issue" in cmd
        assert "comment" in cmd
        assert "42" in cmd
        assert (
            "test comment"
            in [cmd[i] for i in range(len(cmd)) if cmd[i - 1] == "--body"][0]
            if "--body" in cmd
            else True
        )

    @patch("export_to_github.run_gh")
    def test_passes_repo(self, mock_run_gh):
        mock_run_gh.return_value = GhResult(
            success=True, stdout="", stderr="", command=[]
        )
        post_feedback_comment(issue_number=1, body="x", repo="owner/repo")
        cmd = mock_run_gh.call_args[0][0]
        assert "--repo" in cmd
        repo_idx = cmd.index("--repo")
        assert cmd[repo_idx + 1] == "owner/repo"

    @patch("export_to_github.run_gh")
    def test_failure_returns_false(self, mock_run_gh):
        mock_run_gh.return_value = GhResult(
            success=False, stdout="", stderr="error", command=[]
        )
        result = post_feedback_comment(issue_number=1, body="x", repo="o/r")
        assert result is False

    @patch("export_to_github.run_gh")
    def test_dry_run(self, mock_run_gh):
        mock_run_gh.return_value = GhResult(
            success=True, stdout="", stderr="", command=[], dry_run=True
        )
        result = post_feedback_comment(
            issue_number=1, body="x", repo="o/r", dry_run=True
        )
        assert result is True


# ---------------------------------------------------------------------------
# FeedbackSummary
# ---------------------------------------------------------------------------


class TestFeedbackSummary:
    def test_initial_counts(self):
        s = FeedbackSummary()
        assert s.exported == 0
        assert s.skipped == 0
        assert s.failed == 0

    def test_total(self):
        s = FeedbackSummary(exported=3, skipped=2, failed=1)
        assert s.total() == 6

    def test_report(self):
        s = FeedbackSummary(exported=5, skipped=3, failed=1)
        r = s.report()
        assert "Exported:  5" in r
        assert "Skipped:   3" in r
        assert "Failed:    1" in r
        assert "Total:     9" in r


# ---------------------------------------------------------------------------
# export_feedback (orchestrator)
# ---------------------------------------------------------------------------


class TestExportFeedback:
    @patch("export_to_github.post_feedback_comment")
    def test_exports_new_feedback(self, mock_post):
        """New feedback that hasn't been exported is posted as a comment."""
        mock_post.return_value = True
        fb = _make_feedback(to_id="s-xyz", content="New insight")
        spec_data = _make_spec(
            "s-xyz",
            external_links=[_make_ext_link(issue_number=42)],
        )
        entities = [
            ("s-xyz", "spec", spec_data),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        ref_mapping = {"s-xyz": 42}

        summary = export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
        )

        assert summary.exported == 1
        assert mock_post.called

    @patch("export_to_github.post_feedback_comment")
    def test_skips_already_exported_feedback(self, mock_post):
        """Feedback with matching hash in exported_feedback[] is skipped."""
        content = "Already exported content"
        content_hash = compute_feedback_hash(content)
        fb = _make_feedback(to_id="s-xyz", content=content)
        spec_data = _make_spec(
            "s-xyz",
            external_links=[
                _make_ext_link(
                    issue_number=42,
                    exported_feedback=[content_hash],
                )
            ],
        )
        entities = [
            ("s-xyz", "spec", spec_data),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        ref_mapping = {"s-xyz": 42}

        summary = export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
        )

        assert summary.skipped == 1
        assert summary.exported == 0
        assert not mock_post.called

    @patch("export_to_github.post_feedback_comment")
    def test_incremental_exports_new_only(self, mock_post):
        """When re-running, only new feedback is exported."""
        mock_post.return_value = True
        old_content = "Old feedback"
        new_content = "New feedback"
        old_hash = compute_feedback_hash(old_content)

        fb_old = _make_feedback(fb_id="fb-1", to_id="s-xyz", content=old_content)
        fb_new = _make_feedback(fb_id="fb-2", to_id="s-xyz", content=new_content)

        spec_data = _make_spec(
            "s-xyz",
            external_links=[
                _make_ext_link(issue_number=42, exported_feedback=[old_hash])
            ],
        )
        entities = [
            ("s-xyz", "spec", spec_data),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb_old, fb_new])),
        ]
        ref_mapping = {"s-xyz": 42}

        summary = export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
        )

        assert summary.exported == 1
        assert summary.skipped == 1

    @patch("export_to_github.post_feedback_comment")
    def test_failure_increments_failed(self, mock_post):
        mock_post.return_value = False
        fb = _make_feedback(to_id="s-xyz", content="Will fail")
        spec_data = _make_spec(
            "s-xyz",
            external_links=[_make_ext_link(issue_number=42)],
        )
        entities = [
            ("s-xyz", "spec", spec_data),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        ref_mapping = {"s-xyz": 42}

        summary = export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
        )

        assert summary.failed == 1
        assert summary.exported == 0

    @patch("export_to_github.post_feedback_comment")
    def test_spec_not_in_ref_mapping_skipped(self, mock_post):
        """Feedback targeting a spec without a GitHub issue number is skipped."""
        fb = _make_feedback(to_id="s-xyz", content="Some feedback")
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        ref_mapping = {}  # s-xyz not in mapping

        summary = export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
        )

        assert summary.skipped == 1
        assert not mock_post.called

    @patch("export_to_github.post_feedback_comment")
    def test_no_external_link_still_exports(self, mock_post):
        """If spec has no external_link yet, feedback can still be exported
        (dedup check treats missing exported_feedback as empty)."""
        mock_post.return_value = True
        fb = _make_feedback(to_id="s-xyz", content="Feedback")
        spec_data = _make_spec("s-xyz")  # no external_links
        entities = [
            ("s-xyz", "spec", spec_data),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        ref_mapping = {"s-xyz": 42}

        summary = export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
        )

        assert summary.exported == 1

    @patch("export_to_github.post_feedback_comment")
    def test_dry_run_passed_through(self, mock_post):
        """Dry-run flag is passed to post_feedback_comment."""
        mock_post.return_value = True
        fb = _make_feedback(to_id="s-xyz", content="Test")
        spec_data = _make_spec(
            "s-xyz",
            external_links=[_make_ext_link(issue_number=42)],
        )
        entities = [
            ("s-xyz", "spec", spec_data),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        ref_mapping = {"s-xyz": 42}

        export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
            dry_run=True,
        )

        assert mock_post.called
        _, kwargs = mock_post.call_args
        assert kwargs.get("dry_run") is True

    @patch("export_to_github.update_external_link")
    @patch("export_to_github.post_feedback_comment")
    def test_updates_exported_feedback_in_jsonl(self, mock_post, mock_update_link):
        """After successful export, exported_feedback hash is appended
        and written back via CLI."""
        mock_post.return_value = True
        content = "Track this feedback"
        fb = _make_feedback(to_id="s-xyz", content=content)
        spec_data = _make_spec(
            "s-xyz",
            external_links=[_make_ext_link(issue_number=42)],
        )
        entities = [
            ("s-xyz", "spec", spec_data),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        ref_mapping = {"s-xyz": 42}

        summary = export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
            project_id="/tmp/fake",
        )

        assert summary.exported == 1
        # Verify update_external_link was called with updated metadata
        assert mock_update_link.called
        call_args = mock_update_link.call_args
        # update_external_link(project_id, entity_id, external_id, metadata=...)
        call_kwargs = call_args[1]  # keyword args
        metadata = call_kwargs["metadata"]
        expected_hash = compute_feedback_hash(content)
        assert expected_hash in metadata["exported_feedback"]

    @patch("export_to_github.update_external_link")
    @patch("export_to_github.post_feedback_comment")
    def test_no_jsonl_write_on_dry_run(self, mock_post, mock_update_link):
        """Dry-run mode does not write external link updates."""
        mock_post.return_value = True
        fb = _make_feedback(to_id="s-xyz", content="Test")
        spec_data = _make_spec(
            "s-xyz",
            external_links=[_make_ext_link(issue_number=42)],
        )
        entities = [
            ("s-xyz", "spec", spec_data),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb])),
        ]
        ref_mapping = {"s-xyz": 42}

        export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
            dry_run=True,
        )

        assert not mock_update_link.called

    @patch("export_to_github.post_feedback_comment")
    def test_empty_feedback_set(self, mock_post):
        """No feedback to export results in zero counts."""
        entities = [
            ("s-xyz", "spec", _make_spec("s-xyz")),
        ]
        ref_mapping = {"s-xyz": 42}

        summary = export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
        )

        assert summary.total() == 0
        assert not mock_post.called

    @patch("export_to_github.post_feedback_comment")
    def test_partial_failure_continues(self, mock_post):
        """Failure on one feedback entry does not stop processing others."""
        mock_post.side_effect = [False, True]
        fb1 = _make_feedback(fb_id="fb-1", to_id="s-xyz", content="Fail")
        fb2 = _make_feedback(fb_id="fb-2", to_id="s-xyz", content="Succeed")
        spec_data = _make_spec(
            "s-xyz",
            external_links=[_make_ext_link(issue_number=42)],
        )
        entities = [
            ("s-xyz", "spec", spec_data),
            ("i-abc", "issue", _make_issue("i-abc", feedback=[fb1, fb2])),
        ]
        ref_mapping = {"s-xyz": 42}

        summary = export_feedback(
            entities=entities,
            ref_mapping=ref_mapping,
            owner="owner",
            repo="repo",
        )

        assert summary.failed == 1
        assert summary.exported == 1
        assert mock_post.call_count == 2
