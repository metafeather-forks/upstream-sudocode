# /// script
# requires-python = ">=3.11"
# dependencies = ["pytest"]
# ///
"""Tests for CLI scaffold, startup checks, run_gh(), dry-run, and summary (Phase 3).

Acceptance criteria from i-8hq0:
- `uv run export_to_github.py --help` shows usage with all flags
- Script fails gracefully if `gh` is not authenticated
- Script fails gracefully if repo doesn't exist
- Script uses only stdlib modules
- `run_gh()` retries on 429 with exponential backoff
- `--dry-run` produces readable output showing planned actions without making API calls
"""

import json
import subprocess
import sys
import time
from pathlib import Path
from unittest.mock import MagicMock, call, patch

import pytest

sys.path.insert(0, str(Path(__file__).parent.parent / "scripts"))
from export_to_github import (
    ExportSummary,
    GhResult,
    SudocodeResult,
    build_parser,
    check_gh_auth,
    check_repo_exists,
    check_project,
    run_gh,
)


# ---------------------------------------------------------------------------
# Test: GhResult dataclass
# ---------------------------------------------------------------------------


class TestGhResult:
    def test_success_result(self):
        r = GhResult(
            success=True, stdout="output", stderr="", command=["gh", "auth", "status"]
        )
        assert r.success is True
        assert r.stdout == "output"
        assert r.stderr == ""
        assert r.command == ["gh", "auth", "status"]

    def test_failure_result(self):
        r = GhResult(
            success=False, stdout="", stderr="error msg", command=["gh", "repo", "view"]
        )
        assert r.success is False
        assert r.stderr == "error msg"

    def test_dry_run_result(self):
        r = GhResult(
            success=True,
            stdout="",
            stderr="",
            command=["gh", "issue", "create"],
            dry_run=True,
        )
        assert r.dry_run is True


# ---------------------------------------------------------------------------
# Test: ExportSummary
# ---------------------------------------------------------------------------


class TestExportSummary:
    def test_initial_counts(self):
        s = ExportSummary()
        assert s.created == 0
        assert s.updated == 0
        assert s.skipped == 0
        assert s.failed == 0

    def test_increment(self):
        s = ExportSummary()
        s.created += 1
        s.updated += 2
        s.skipped += 3
        s.failed += 1
        assert s.created == 1
        assert s.updated == 2
        assert s.skipped == 3
        assert s.failed == 1

    def test_total(self):
        s = ExportSummary()
        s.created = 5
        s.updated = 3
        s.skipped = 2
        s.failed = 1
        assert s.total() == 11

    def test_report_includes_counts(self):
        s = ExportSummary()
        s.created = 2
        s.updated = 1
        s.skipped = 3
        s.failed = 0
        report = s.report()
        assert "Created: 2" in report
        assert "Updated: 1" in report
        assert "Skipped: 3" in report
        assert "Failed:" in report and "0" in report
        assert "Total:" in report and "6" in report


# ---------------------------------------------------------------------------
# Test: CLI argument parsing
# ---------------------------------------------------------------------------


class TestBuildParser:
    def test_required_args(self):
        parser = build_parser()
        args = parser.parse_args(["--spec-id", "s-1234", "--repo", "owner/repo", "--project-id", "my-proj-abc123"])
        assert args.spec_id == "s-1234"
        assert args.repo == "owner/repo"
        assert args.project_id == "my-proj-abc123"

    def test_missing_spec_id_fails(self):
        parser = build_parser()
        with pytest.raises(SystemExit):
            parser.parse_args(["--repo", "owner/repo", "--project-id", "my-proj-abc123"])

    def test_missing_repo_fails(self):
        parser = build_parser()
        with pytest.raises(SystemExit):
            parser.parse_args(["--spec-id", "s-1234", "--project-id", "my-proj-abc123"])

    def test_missing_project_id_fails(self):
        parser = build_parser()
        with pytest.raises(SystemExit):
            parser.parse_args(["--spec-id", "s-1234", "--repo", "owner/repo"])

    def test_project_id_custom(self):
        parser = build_parser()
        args = parser.parse_args(
            [
                "--spec-id",
                "s-1234",
                "--repo",
                "owner/repo",
                "--project-id",
                "custom-proj-xyz789",
            ]
        )
        assert args.project_id == "custom-proj-xyz789"

    def test_spec_label_default(self):
        parser = build_parser()
        args = parser.parse_args(["--spec-id", "s-1234", "--repo", "owner/repo", "--project-id", "p1"])
        assert args.spec_label == "spec"

    def test_spec_label_custom(self):
        parser = build_parser()
        args = parser.parse_args(
            [
                "--spec-id",
                "s-1234",
                "--repo",
                "owner/repo",
                "--project-id",
                "p1",
                "--spec-label",
                "specification",
            ]
        )
        assert args.spec_label == "specification"

    def test_issue_label_default(self):
        parser = build_parser()
        args = parser.parse_args(["--spec-id", "s-1234", "--repo", "owner/repo", "--project-id", "p1"])
        assert args.issue_label == ""

    def test_issue_label_custom(self):
        parser = build_parser()
        args = parser.parse_args(
            [
                "--spec-id",
                "s-1234",
                "--repo",
                "owner/repo",
                "--project-id",
                "p1",
                "--issue-label",
                "work-item",
            ]
        )
        assert args.issue_label == "work-item"

    def test_dry_run_flag(self):
        parser = build_parser()
        args = parser.parse_args(
            ["--spec-id", "s-1234", "--repo", "owner/repo", "--project-id", "p1", "--dry-run"]
        )
        assert args.dry_run is True

    def test_dry_run_default_false(self):
        parser = build_parser()
        args = parser.parse_args(["--spec-id", "s-1234", "--repo", "owner/repo", "--project-id", "p1"])
        assert args.dry_run is False

    def test_force_flag(self):
        parser = build_parser()
        args = parser.parse_args(
            ["--spec-id", "s-1234", "--repo", "owner/repo", "--project-id", "p1", "--force"]
        )
        assert args.force is True

    def test_force_default_false(self):
        parser = build_parser()
        args = parser.parse_args(["--spec-id", "s-1234", "--repo", "owner/repo", "--project-id", "p1"])
        assert args.force is False

    def test_delay_flag(self):
        parser = build_parser()
        args = parser.parse_args(
            ["--spec-id", "s-1234", "--repo", "owner/repo", "--project-id", "p1", "--delay", "2.5"]
        )
        assert args.delay == 2.5

    def test_delay_default_one(self):
        parser = build_parser()
        args = parser.parse_args(["--spec-id", "s-1234", "--repo", "owner/repo", "--project-id", "p1"])
        assert args.delay == 1.0

    def test_all_flags_combined(self):
        parser = build_parser()
        args = parser.parse_args(
            [
                "--spec-id",
                "s-2a7c",
                "--repo",
                "org/project",
                "--project-id",
                "my-proj-abc123",
                "--spec-label",
                "requirement",
                "--issue-label",
                "task",
                "--dry-run",
                "--force",
                "--delay",
                "0.5",
            ]
        )
        assert args.spec_id == "s-2a7c"
        assert args.repo == "org/project"
        assert args.project_id == "my-proj-abc123"
        assert args.spec_label == "requirement"
        assert args.issue_label == "task"
        assert args.dry_run is True
        assert args.force is True
        assert args.delay == 0.5


# ---------------------------------------------------------------------------
# Test: run_gh() helper
# ---------------------------------------------------------------------------


class TestRunGh:
    @patch("export_to_github.subprocess.run")
    def test_successful_command(self, mock_run):
        mock_run.return_value = subprocess.CompletedProcess(
            args=["gh", "auth", "status"],
            returncode=0,
            stdout="Logged in",
            stderr="",
        )
        result = run_gh(["gh", "auth", "status"])
        assert result.success is True
        assert result.stdout == "Logged in"
        mock_run.assert_called_once()

    @patch("export_to_github.subprocess.run")
    def test_failed_command(self, mock_run):
        mock_run.return_value = subprocess.CompletedProcess(
            args=["gh", "repo", "view", "bad/repo"],
            returncode=1,
            stdout="",
            stderr="Could not resolve",
        )
        result = run_gh(["gh", "repo", "view", "bad/repo"])
        assert result.success is False
        assert "Could not resolve" in result.stderr

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_retry_on_429(self, mock_sleep, mock_run):
        """HTTP 429 in stderr triggers retry with exponential backoff."""
        # First call: 429 error, second call: success
        mock_run.side_effect = [
            subprocess.CompletedProcess(
                args=["gh", "api", "..."],
                returncode=1,
                stdout="",
                stderr="HTTP 429",
            ),
            subprocess.CompletedProcess(
                args=["gh", "api", "..."],
                returncode=0,
                stdout="success",
                stderr="",
            ),
        ]
        result = run_gh(["gh", "api", "..."])
        assert result.success is True
        assert result.stdout == "success"
        assert mock_run.call_count == 2
        # Should have slept between retries
        mock_sleep.assert_called_once()

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_exponential_backoff_delays(self, mock_sleep, mock_run):
        """Backoff delays should increase exponentially: 1s, 2s, 4s, 8s, 16s."""
        # All calls return 429 except the last
        mock_run.side_effect = [
            subprocess.CompletedProcess(
                args=["gh"], returncode=1, stdout="", stderr="HTTP 429"
            )
            for _ in range(3)
        ] + [
            subprocess.CompletedProcess(
                args=["gh"], returncode=0, stdout="ok", stderr=""
            )
        ]
        result = run_gh(["gh", "api", "test"])
        assert result.success is True
        assert mock_run.call_count == 4
        # Check exponential delays: 1, 2, 4
        sleep_calls = [c[0][0] for c in mock_sleep.call_args_list]
        assert sleep_calls == [1, 2, 4]

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_max_retries_exhausted(self, mock_sleep, mock_run):
        """After max retries, returns the last failure result."""
        mock_run.return_value = subprocess.CompletedProcess(
            args=["gh"], returncode=1, stdout="", stderr="HTTP 429"
        )
        result = run_gh(["gh", "api", "test"], max_retries=5)
        assert result.success is False
        assert "429" in result.stderr
        # 1 initial + 5 retries = 6 total calls
        assert mock_run.call_count == 6

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_backoff_capped_at_max(self, mock_sleep, mock_run):
        """Backoff delay is capped at max_backoff (60s)."""
        # Need enough retries for delay to exceed 60s: 1, 2, 4, 8, 16, 32, 64->60
        mock_run.return_value = subprocess.CompletedProcess(
            args=["gh"], returncode=1, stdout="", stderr="HTTP 429"
        )
        run_gh(["gh", "api", "test"], max_retries=8, max_backoff=60)
        sleep_calls = [c[0][0] for c in mock_sleep.call_args_list]
        # All delays should be <= 60
        for delay in sleep_calls:
            assert delay <= 60

    @patch("export_to_github.subprocess.run")
    def test_non_retryable_error_not_retried(self, mock_run):
        """Non-retryable errors (e.g. permission denied) are NOT retried."""
        mock_run.return_value = subprocess.CompletedProcess(
            args=["gh"], returncode=1, stdout="", stderr="Permission denied"
        )
        result = run_gh(["gh", "api", "test"])
        assert result.success is False
        assert mock_run.call_count == 1  # No retry

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_retry_on_404(self, mock_sleep, mock_run):
        """HTTP 404 in stderr triggers retry (eventual consistency)."""
        mock_run.side_effect = [
            subprocess.CompletedProcess(
                args=["gh"], returncode=1, stdout="", stderr="HTTP 404"
            ),
            subprocess.CompletedProcess(
                args=["gh"], returncode=0, stdout="success", stderr=""
            ),
        ]
        result = run_gh(["gh", "api", "test"])
        assert result.success is True
        assert mock_run.call_count == 2
        mock_sleep.assert_called_once()

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_retry_on_500(self, mock_sleep, mock_run):
        """HTTP 500 in stderr triggers retry."""
        mock_run.side_effect = [
            subprocess.CompletedProcess(
                args=["gh"], returncode=1, stdout="", stderr="HTTP 500"
            ),
            subprocess.CompletedProcess(
                args=["gh"], returncode=0, stdout="success", stderr=""
            ),
        ]
        result = run_gh(["gh", "api", "test"])
        assert result.success is True
        assert mock_run.call_count == 2

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_retry_on_502(self, mock_sleep, mock_run):
        """HTTP 502 in stderr triggers retry."""
        mock_run.side_effect = [
            subprocess.CompletedProcess(
                args=["gh"], returncode=1, stdout="", stderr="HTTP 502"
            ),
            subprocess.CompletedProcess(
                args=["gh"], returncode=0, stdout="success", stderr=""
            ),
        ]
        result = run_gh(["gh", "api", "test"])
        assert result.success is True
        assert mock_run.call_count == 2

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_retry_on_503(self, mock_sleep, mock_run):
        """HTTP 503 in stderr triggers retry."""
        mock_run.side_effect = [
            subprocess.CompletedProcess(
                args=["gh"], returncode=1, stdout="", stderr="HTTP 503"
            ),
            subprocess.CompletedProcess(
                args=["gh"], returncode=0, stdout="success", stderr=""
            ),
        ]
        result = run_gh(["gh", "api", "test"])
        assert result.success is True
        assert mock_run.call_count == 2

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_retry_on_connection_error(self, mock_sleep, mock_run):
        """Connection errors in stderr trigger retry."""
        mock_run.side_effect = [
            subprocess.CompletedProcess(
                args=["gh"], returncode=1, stdout="", stderr="connection reset by peer"
            ),
            subprocess.CompletedProcess(
                args=["gh"], returncode=0, stdout="success", stderr=""
            ),
        ]
        result = run_gh(["gh", "api", "test"])
        assert result.success is True
        assert mock_run.call_count == 2

    @patch("export_to_github.subprocess.run")
    @patch("export_to_github.time.sleep")
    def test_retry_on_timeout(self, mock_sleep, mock_run):
        """Timeout errors in stderr trigger retry."""
        mock_run.side_effect = [
            subprocess.CompletedProcess(
                args=["gh"], returncode=1, stdout="", stderr="timeout exceeded"
            ),
            subprocess.CompletedProcess(
                args=["gh"], returncode=0, stdout="success", stderr=""
            ),
        ]
        result = run_gh(["gh", "api", "test"])
        assert result.success is True
        assert mock_run.call_count == 2

    @patch("export_to_github.subprocess.run")
    def test_dry_run_skips_execution(self, mock_run):
        """In dry-run mode, command is not executed."""
        result = run_gh(["gh", "issue", "create", "--title", "test"], dry_run=True)
        assert result.success is True
        assert result.dry_run is True
        mock_run.assert_not_called()

    @patch("export_to_github.subprocess.run")
    def test_command_stored_in_result(self, mock_run):
        """The command is stored in the result."""
        mock_run.return_value = subprocess.CompletedProcess(
            args=["gh", "auth", "status"], returncode=0, stdout="ok", stderr=""
        )
        result = run_gh(["gh", "auth", "status"])
        assert result.command == ["gh", "auth", "status"]


# ---------------------------------------------------------------------------
# Test: Startup checks
# ---------------------------------------------------------------------------


class TestCheckGhAuth:
    @patch("export_to_github.run_gh")
    def test_auth_success(self, mock_run_gh):
        mock_run_gh.return_value = GhResult(
            success=True,
            stdout="Logged in to github.com",
            stderr="",
            command=["gh", "auth", "status"],
        )
        # Should not raise
        check_gh_auth()
        mock_run_gh.assert_called_once()

    @patch("export_to_github.run_gh")
    def test_auth_failure(self, mock_run_gh):
        mock_run_gh.return_value = GhResult(
            success=False,
            stdout="",
            stderr="not logged in",
            command=["gh", "auth", "status"],
        )
        with pytest.raises(SystemExit):
            check_gh_auth()


class TestCheckRepoExists:
    @patch("export_to_github.run_gh")
    def test_repo_exists(self, mock_run_gh):
        mock_run_gh.return_value = GhResult(
            success=True, stdout="owner/repo", stderr="", command=["gh", "repo", "view"]
        )
        check_repo_exists("owner/repo")

    @patch("export_to_github.run_gh")
    def test_repo_not_found(self, mock_run_gh):
        mock_run_gh.return_value = GhResult(
            success=False,
            stdout="",
            stderr="Could not resolve",
            command=["gh", "repo", "view"],
        )
        with pytest.raises(SystemExit):
            check_repo_exists("invalid/repo")


class TestCheckProject:
    @patch("export_to_github.run_sudocode")
    def test_valid_project(self, mock_run):
        """Successful sudocode status check does not raise."""
        mock_run.return_value = SudocodeResult(
            success=True, stdout="Project OK", stderr="", command=["sudocode", "status"]
        )
        # Should not raise
        check_project("my-proj-abc123")
        mock_run.assert_called_once()

    @patch("export_to_github.run_sudocode")
    def test_missing_project(self, mock_run):
        """Failed sudocode status raises SystemExit."""
        mock_run.return_value = SudocodeResult(
            success=False,
            stdout="",
            stderr="Project not found",
            command=["sudocode", "status"],
        )
        with pytest.raises(SystemExit):
            check_project("nonexistent-proj-000")

    @patch("export_to_github.run_sudocode")
    def test_cli_failure_raises(self, mock_run):
        """Any CLI failure raises SystemExit."""
        mock_run.return_value = SudocodeResult(
            success=False,
            stdout="",
            stderr="database error",
            command=["sudocode", "status"],
        )
        with pytest.raises(SystemExit):
            check_project("my-proj-abc123")
