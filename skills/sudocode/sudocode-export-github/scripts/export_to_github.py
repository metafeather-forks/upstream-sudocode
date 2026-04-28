# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""
Export Sudocode specs and issues to GitHub Issues.

Phase 1: Graph collection and topological sort.
Phase 2: Reference rewriting and content hashing.
Phase 3: CLI interface, startup checks, and dry-run mode.
Phase 4: GitHub Issue creation/update with external_links tracking.
Phase 5: GitHub relationship establishment (sub-issues, dependencies, references).
Phase 6: Feedback export as GitHub Issue comments.

Uses the ``sudocode`` CLI to load entity data (via ``sudocode export``)
and to manage external links (via ``sudocode external-link add/update``).
Collects the full dependency graph from a root spec, produces a
topologically sorted export order, rewrites [[id]] references to
GitHub #number format, and computes content hashes for incremental sync.
Creates/updates GitHub Issues and tracks mappings via the sudocode
external-link API.  Establishes relationships between GitHub Issues
using the sub-issues, dependencies, and comment APIs.  Exports feedback
entries as comments on the corresponding GitHub Issues.

JSONL field naming convention (differs from SQLite/runtime):
  - relationships[].from  (not from_id)
  - relationships[].to    (not to_id)
  - relationships[].type  (not relationship_type)
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
import tempfile
import time
from collections import defaultdict, deque
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

# Relationship types that create ordering constraints in topological sort.
# "blocks": from blocks to  -> from before to
# "parent-child": parent before child
# "implements": spec before implementing issue (reversed: issue->spec edge
#               means spec must come first)
# "depends-on": from depends-on to -> to before from (reversed direction)
ORDERING_EDGE_TYPES = frozenset({"blocks", "parent-child", "implements", "depends-on"})

# Relationship types that are captured but do NOT create ordering constraints.
NON_ORDERING_EDGE_TYPES = frozenset({"references", "related", "discovered-from"})

# Priority of ordering edge types for cycle-breaking.  Lower number = stronger
# (harder to break).  When a cycle is detected, the edge with the *highest*
# priority value (weakest) is removed from the ordering graph. The removed edge
# is still established as a GitHub relationship — it just doesn't constrain
# export order.
EDGE_PRIORITY: dict[str, int] = {
    "blocks": 0,  # strongest — explicit execution ordering
    "depends-on": 1,
    "implements": 2,
    "parent-child": 3,  # weakest — organisational hierarchy
}


class CyclicDependencyError(Exception):
    """Raised when a circular dependency is detected during topological sort."""

    def __init__(self, message: str, cycle: list[str] | None = None):
        super().__init__(message)
        self.cycle = cycle


# ---------------------------------------------------------------------------
# sudocode CLI helper
# ---------------------------------------------------------------------------


@dataclass
class SudocodeResult:
    """Result from a ``sudocode`` CLI invocation."""

    success: bool
    stdout: str
    stderr: str
    command: list[str]
    dry_run: bool = False


def run_sudocode(
    command: list[str],
    *,
    sudocode_dir: str | Path | None = None,
    dry_run: bool = False,
) -> SudocodeResult:
    """Execute a sudocode CLI command.

    Args:
        command: Command list WITHOUT the leading ``sudocode``, e.g.
            ``["spec", "list", "--json"]``.
        sudocode_dir: Working directory for project discovery.  Passed as
            ``--working-dir`` to the CLI.
        dry_run: If True, skip execution and return a dry-run result.

    Returns:
        SudocodeResult with success/failure status and captured output.
    """
    full_cmd = ["sudocode"]
    if sudocode_dir is not None:
        full_cmd += ["--working-dir", str(sudocode_dir)]
    full_cmd += command

    if dry_run:
        print(f"[dry-run] Would execute: {' '.join(full_cmd)}")
        return SudocodeResult(
            success=True,
            stdout="",
            stderr="",
            command=full_cmd,
            dry_run=True,
        )

    proc = subprocess.run(
        full_cmd,
        capture_output=True,
        text=True,
    )

    return SudocodeResult(
        success=proc.returncode == 0,
        stdout=proc.stdout,
        stderr=proc.stderr,
        command=full_cmd,
    )


# ---------------------------------------------------------------------------
# Entity loading via sudocode CLI
# ---------------------------------------------------------------------------


def _parse_jsonl(path: Path) -> dict[str, dict[str, Any]]:
    """Parse a JSONL file into a dict keyed by entity ``id``."""
    result: dict[str, dict[str, Any]] = {}
    if not path.exists():
        return result
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            obj = json.loads(line)
            result[obj["id"]] = obj
    return result


def load_entities(
    sudocode_dir: str | Path,
) -> tuple[dict[str, dict[str, Any]], dict[str, dict[str, Any]]]:
    """Load all specs and issues via ``sudocode export``.

    Runs ``sudocode export -o <tmpdir>`` to get a consistent snapshot from
    the database, then parses the exported JSONL files.  This avoids
    reading the live ``.sudocode/*.jsonl`` files directly (which may be
    out of sync with the SQLite database).

    Args:
        sudocode_dir: Path to the ``.sudocode`` directory (or its parent).
            Passed as ``--working-dir`` to the CLI.

    Returns:
        Tuple of ``(specs_dict, issues_dict)`` where each dict maps
        entity ID to entity data (including ``relationships``,
        ``external_links``, etc.).

    Raises:
        RuntimeError: if the export command fails.
    """
    with tempfile.TemporaryDirectory(prefix="sudocode-export-") as tmpdir:
        result = run_sudocode(
            ["export", "-o", tmpdir],
            sudocode_dir=sudocode_dir,
        )
        if not result.success:
            raise RuntimeError(f"sudocode export failed: {result.stderr.strip()}")

        specs = _parse_jsonl(Path(tmpdir) / "specs.jsonl")
        issues = _parse_jsonl(Path(tmpdir) / "issues.jsonl")

    return specs, issues


# ---------------------------------------------------------------------------
# Graph collection
# ---------------------------------------------------------------------------


def collect_graph(
    spec_id: str,
    specs: dict[str, dict[str, Any]],
    issues: dict[str, dict[str, Any]],
) -> tuple[list[tuple[str, str, dict]], list[tuple[str, str, str]]]:
    """Collect the full dependency graph starting from a spec.

    Algorithm:
    1. Start with the target spec.
    2. Find all child specs (parent_id == spec_id), recursively.
    3. For each collected spec, find issues that implement it.
    4. Capture all edges between collected entities.

    Returns:
        entities: list of (entity_id, entity_type, entity_data) tuples
        edges: list of (from_id, to_id, relationship_type) tuples
    """
    if spec_id not in specs:
        raise KeyError(f"Spec not found: {spec_id}")

    # Sets of collected entity IDs
    collected_ids: set[str] = set()

    # Pre-build indexes for efficient lookups
    # Index: parent_id -> list of child spec IDs
    spec_children: dict[str, list[str]] = defaultdict(list)
    for sid, sdata in specs.items():
        pid = sdata.get("parent_id")
        if pid:
            spec_children[pid].append(sid)

    # Index: parent_id -> list of child issue IDs
    issue_children: dict[str, list[str]] = defaultdict(list)
    for iid, idata in issues.items():
        pid = idata.get("parent_id")
        if pid:
            issue_children[pid].append(iid)

    # Index: spec_id -> list of issue IDs that implement it
    implementing_issues: dict[str, list[str]] = defaultdict(list)
    for iid, idata in issues.items():
        for rel in idata.get("relationships", []):
            if (
                rel.get("type") == "implements"
                and rel.get("to_type") == "spec"
                and rel.get("from") == iid
            ):
                implementing_issues[rel["to"]].append(iid)

    # Phase 1: Collect specs (root + transitive children)
    def _collect_spec(sid: str) -> None:
        if sid in collected_ids:
            return
        collected_ids.add(sid)
        # Recurse into children
        for child_sid in spec_children.get(sid, []):
            _collect_spec(child_sid)

    _collect_spec(spec_id)

    # Phase 2: Collect implementing issues for all collected specs,
    # then recursively collect their child issues (Fix 3).
    def _collect_issue_tree(iid: str) -> None:
        if iid in collected_ids:
            return
        collected_ids.add(iid)
        for child_iid in issue_children.get(iid, []):
            _collect_issue_tree(child_iid)

    collected_spec_ids = set(collected_ids)  # snapshot before adding issues
    for sid in collected_spec_ids:
        for iid in implementing_issues.get(sid, []):
            _collect_issue_tree(iid)

    # Phase 3: Build entity list and edge list
    entities: list[tuple[str, str, dict]] = []
    edges: list[tuple[str, str, str]] = []

    for eid in collected_ids:
        if eid in specs:
            entities.append((eid, "spec", specs[eid]))
        elif eid in issues:
            entities.append((eid, "issue", issues[eid]))

    # Add parent-child edges for specs
    for eid in collected_ids:
        if eid in specs:
            pid = specs[eid].get("parent_id")
            if pid and pid in collected_ids:
                edges.append((pid, eid, "parent-child"))

    # Add parent-child edges for issues
    for eid in collected_ids:
        if eid in issues:
            pid = issues[eid].get("parent_id")
            if pid and pid in collected_ids:
                edges.append((pid, eid, "parent-child"))

    # Add relationship edges (from JSONL relationships array)
    for eid in collected_ids:
        entity = specs.get(eid) or issues.get(eid)
        if not entity:
            continue
        for rel in entity.get("relationships", []):
            from_id = rel.get("from")
            to_id = rel.get("to")
            rtype = rel.get("type")
            # Only include edges where BOTH endpoints are in the collected graph
            if from_id in collected_ids and to_id in collected_ids:
                edges.append((from_id, to_id, rtype))

    # Deduplicate edges
    edges = list(set(edges))

    return entities, edges


# ---------------------------------------------------------------------------
# Topological sort (Kahn's algorithm)
# ---------------------------------------------------------------------------


def _find_cycles(
    adjacency: dict[str, list[str]], in_degree: dict[str, int]
) -> list[str]:
    """Find a cycle in the graph using DFS from nodes with remaining in-degree > 0."""
    # Nodes still in the cycle are those with in-degree > 0 after Kahn's
    remaining = {n for n, d in in_degree.items() if d > 0}
    if not remaining:
        return []

    # DFS to find a cycle path
    visited: set[str] = set()
    rec_stack: list[str] = []
    rec_set: set[str] = set()

    def _dfs(node: str) -> list[str] | None:
        visited.add(node)
        rec_stack.append(node)
        rec_set.add(node)

        for neighbor in adjacency.get(node, []):
            if neighbor not in remaining:
                continue
            if neighbor in rec_set:
                # Found cycle - extract it
                cycle_start = rec_stack.index(neighbor)
                return rec_stack[cycle_start:]
            if neighbor not in visited:
                result = _dfs(neighbor)
                if result is not None:
                    return result

        rec_stack.pop()
        rec_set.discard(node)
        return None

    for node in remaining:
        if node not in visited:
            cycle = _dfs(node)
            if cycle is not None:
                return cycle

    # Fallback: return all remaining nodes if DFS didn't find a clean cycle
    return list(remaining)


def _ordering_direction(from_id: str, to_id: str, rtype: str) -> tuple[str, str]:
    """Return (prerequisite, dependent) for an ordering edge.

    Translates the raw edge direction into the actual ordering direction
    used in the adjacency graph.
    """
    if rtype in ("blocks", "parent-child"):
        return (from_id, to_id)
    elif rtype == "depends-on":
        return (to_id, from_id)
    elif rtype == "implements":
        return (to_id, from_id)
    raise ValueError(f"Unknown ordering edge type: {rtype}")


def _build_ordering_graph(
    node_ids: set[str],
    ordering_edges: list[tuple[str, str, str]],
) -> tuple[dict[str, list[str]], dict[str, int]]:
    """Build adjacency list and in-degree map from ordering edges.

    Returns (adjacency, in_degree).
    """
    adjacency: dict[str, list[str]] = defaultdict(list)
    in_degree: dict[str, int] = {nid: 0 for nid in node_ids}

    for from_id, to_id, rtype in ordering_edges:
        pre, dep = _ordering_direction(from_id, to_id, rtype)
        adjacency[pre].append(dep)
        in_degree[dep] += 1

    return adjacency, in_degree


def _find_cycle_in_directed_graph(
    adjacency: dict[str, list[str]], node_ids: set[str]
) -> list[str] | None:
    """Find a single cycle in the directed graph using DFS.

    Returns the cycle as a list of node IDs (without the repeated start node),
    or None if no cycle exists.
    """
    visited: set[str] = set()
    rec_stack: list[str] = []
    rec_set: set[str] = set()

    def _dfs(node: str) -> list[str] | None:
        visited.add(node)
        rec_stack.append(node)
        rec_set.add(node)

        for neighbor in adjacency.get(node, []):
            if neighbor not in node_ids:
                continue
            if neighbor in rec_set:
                cycle_start = rec_stack.index(neighbor)
                return rec_stack[cycle_start:]
            if neighbor not in visited:
                result = _dfs(neighbor)
                if result is not None:
                    return result

        rec_stack.pop()
        rec_set.discard(node)
        return None

    for node in node_ids:
        if node not in visited:
            cycle = _dfs(node)
            if cycle is not None:
                return cycle

    return None


def _break_cycles(
    node_ids: set[str],
    ordering_edges: list[tuple[str, str, str]],
) -> tuple[list[tuple[str, str, str]], list[tuple[str, str, str]]]:
    """Iteratively detect and break cycles by removing the weakest edge.

    Uses EDGE_PRIORITY to determine which edge to remove: the edge with the
    highest priority value (weakest strength) in the cycle is removed.

    If a cycle consists entirely of edges with the same (strongest) priority
    (e.g. all ``blocks``), no edge can be broken and the cycle is left for
    Kahn's algorithm to detect and raise ``CyclicDependencyError``.

    Args:
        node_ids: Set of all node IDs in the graph.
        ordering_edges: List of ordering edges (from, to, type).

    Returns:
        Tuple of (remaining_edges, demoted_edges).
        ``remaining_edges`` is the ordering edges minus any that were removed.
        ``demoted_edges`` is the list of edges that were removed to break cycles.
    """
    remaining = list(ordering_edges)
    demoted: list[tuple[str, str, str]] = []

    while True:
        adjacency, _ = _build_ordering_graph(node_ids, remaining)
        cycle = _find_cycle_in_directed_graph(adjacency, node_ids)
        if cycle is None:
            break

        # Find which edges in `remaining` participate in this cycle.
        # The cycle is [n0, n1, ..., nk] meaning n0->n1->...->nk->n0.
        cycle_set = set(cycle)
        cycle_directed_edges: set[tuple[str, str]] = set()
        for i in range(len(cycle)):
            pre = cycle[i]
            dep = cycle[(i + 1) % len(cycle)]
            cycle_directed_edges.add((pre, dep))

        # Find the weakest edge in the cycle
        weakest_idx: int | None = None
        weakest_priority: int = -1

        for idx, (from_id, to_id, rtype) in enumerate(remaining):
            pre, dep = _ordering_direction(from_id, to_id, rtype)
            if (pre, dep) in cycle_directed_edges:
                edge_pri = EDGE_PRIORITY.get(rtype, 0)
                if edge_pri > weakest_priority:
                    weakest_priority = edge_pri
                    weakest_idx = idx

        if weakest_idx is None:
            # Should not happen if cycle detection is correct, but be safe
            break

        # Check if all cycle edges have the same priority as the weakest
        # (i.e. they're all the strongest type like blocks).
        # In that case, we can't break the cycle — leave it for the error.
        cycle_priorities = set()
        for idx, (from_id, to_id, rtype) in enumerate(remaining):
            pre, dep = _ordering_direction(from_id, to_id, rtype)
            if (pre, dep) in cycle_directed_edges:
                cycle_priorities.add(EDGE_PRIORITY.get(rtype, 0))

        if len(cycle_priorities) == 1:
            # All edges in cycle have the same priority — unbreakable
            break

        # Remove the weakest edge
        removed = remaining.pop(weakest_idx)
        demoted.append(removed)

        # Warn the user
        print(
            f"Warning: Cycle detected, demoting '{removed[2]}' edge "
            f"({removed[0]} -> {removed[1]}) from ordering constraints. "
            f"The relationship will still be established on GitHub.",
            file=sys.stderr,
        )

    return remaining, demoted


def topological_sort(
    entities: list[tuple[str, str, dict]],
    edges: list[tuple[str, str, str]],
) -> tuple[list[tuple[str, str, dict]], list[tuple[str, str, str]]]:
    """Topologically sort entities using Kahn's algorithm with cycle-breaking.

    Ordering constraints:
    - parent-child: parent before child (parent -> child)
    - blocks: blocker before blocked (from -> to)
    - depends-on: depended-on before depender (to -> from, reversed)
    - implements: spec before implementing issue (to -> from, reversed)

    Non-ordering edges (references, related, discovered-from) are ignored.

    When cycles are detected, the weakest edge in each cycle is removed
    (demoted) from the ordering graph.  Demoted edges are returned so the
    caller can still establish the corresponding GitHub relationships.

    If a cycle cannot be broken (all edges are the same strongest type),
    ``CyclicDependencyError`` is raised.

    Returns:
        Tuple of (sorted_entities, demoted_edges).
        ``sorted_entities`` is the topologically sorted list.
        ``demoted_edges`` is the list of edges removed to break cycles.
    """
    entity_map = {e[0]: e for e in entities}
    node_ids = set(entity_map.keys())

    # Collect ordering edges only, filtering to known nodes.
    ordering_edges: list[tuple[str, str, str]] = []
    for from_id, to_id, rtype in edges:
        if rtype not in ORDERING_EDGE_TYPES:
            continue
        if from_id not in node_ids or to_id not in node_ids:
            continue
        ordering_edges.append((from_id, to_id, rtype))

    # Break cycles by demoting weak edges
    remaining_edges, demoted_edges = _break_cycles(node_ids, ordering_edges)

    # Build ordering graph from remaining edges
    adjacency, in_degree = _build_ordering_graph(node_ids, remaining_edges)

    # Kahn's algorithm
    queue: deque[str] = deque()
    for nid in node_ids:
        if in_degree[nid] == 0:
            queue.append(nid)

    sorted_ids: list[str] = []

    while queue:
        node = queue.popleft()
        sorted_ids.append(node)

        for neighbor in adjacency.get(node, []):
            in_degree[neighbor] -= 1
            if in_degree[neighbor] == 0:
                queue.append(neighbor)

    if len(sorted_ids) != len(node_ids):
        # Cycle detected even after breaking — find and report
        cycle = _find_cycles(adjacency, in_degree)
        cycle_str = " -> ".join(cycle + [cycle[0]] if cycle else ["unknown"])
        raise CyclicDependencyError(
            f"Circular dependency detected: {cycle_str}",
            cycle=cycle,
        )

    return [entity_map[nid] for nid in sorted_ids], demoted_edges


# ---------------------------------------------------------------------------
# Content hashing
# ---------------------------------------------------------------------------


def compute_content_hash(title: str, content: str | None) -> str:
    """Compute SHA-256 hash of title + content for change detection.

    Matches the server implementation in external-refresh-service.ts:87-92:
    SHA-256 of title + (content or "").

    Args:
        title: Entity title.
        content: Entity content/description. None or "" are treated identically.

    Returns:
        Hex-encoded SHA-256 digest.
    """
    raw = title + (content or "")
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()


# ---------------------------------------------------------------------------
# Reference rewriting
# ---------------------------------------------------------------------------

# Pattern matches [[id]] or [[id|display]] with optional @-prefix on the id,
# optionally followed by { relationship_type }.
#
# Groups:
#   1: optional @ prefix
#   2: entity id  (e.g. "s-1234", "i-abcd")
#   3: optional |display text  (including the pipe)
#   4: display text only (without pipe)
#   5: optional { relationship } suffix (including braces)
_REF_PATTERN = re.compile(
    r"\[\["
    r"(@)?"  # group 1: optional @ prefix
    r"([si]-[A-Za-z0-9]+)"  # group 2: entity id
    r"(?:\|([^\]]*))?"  # group 3: optional display text (without pipe)
    r"\]\]"
    r"(?:\{\s*[a-z-]+\s*\})?"  # optional { relationship } suffix (non-capturing)
)


def rewrite_references(text: str, mapping: dict[str, int]) -> str:
    """Rewrite Sudocode [[id]] references to GitHub #number format.

    Handles:
    - [[s-XXXX]] -> #N (when in mapping)
    - [[s-XXXX|Display Text]] -> Display Text (#N)
    - [[s-unknown]] -> s-unknown (not in mapping, raw ID)
    - [[s-unknown|Text]] -> Text (not in mapping, display text)
    - [[@s-XXXX]] -> same as [[s-XXXX]] (@ prefix stripped)
    - [[s-XXXX]]{ blocks } -> #N (relationship suffix stripped)

    References inside ``` code fences are NOT rewritten.

    Args:
        text: Content string potentially containing [[id]] references.
        mapping: Dict of sudocode_id -> github_issue_number.

    Returns:
        Text with references rewritten.
    """
    if not text:
        return text

    # Split text on code fence boundaries to protect fenced content.
    # Code fences are lines starting with ``` (with optional language tag).
    parts = re.split(r"(```[^\n]*\n.*?```)", text, flags=re.DOTALL)

    result_parts: list[str] = []
    for i, part in enumerate(parts):
        if part.startswith("```"):
            # Inside a code fence — leave untouched
            result_parts.append(part)
        else:
            # Outside code fence — rewrite references
            result_parts.append(_rewrite_refs_in_segment(part, mapping))

    return "".join(result_parts)


def _rewrite_refs_in_segment(text: str, mapping: dict[str, int]) -> str:
    """Rewrite all [[id]] references in a text segment (not inside code fences)."""

    def _replace_match(m: re.Match) -> str:
        entity_id = m.group(2)
        display_text = m.group(3)

        if entity_id in mapping:
            issue_number = mapping[entity_id]
            if display_text:
                return f"{display_text} (#{issue_number})"
            return f"#{issue_number}"
        else:
            # Unknown reference — strip wrapper, keep display text or raw ID
            if display_text:
                return display_text
            return entity_id

    return _REF_PATTERN.sub(_replace_match, text)


# ---------------------------------------------------------------------------
# External link lookup
# ---------------------------------------------------------------------------


def find_external_link(
    entity: dict[str, Any], owner: str, repo: str
) -> dict[str, Any] | None:
    """Find an existing external_link for a given entity/owner/repo.

    Searches the entity's external_links array for an entry where:
    - provider == "github"
    - metadata.owner == owner
    - metadata.repo == repo

    Args:
        entity: Entity dict (spec or issue) from JSONL.
        owner: GitHub repo owner.
        repo: GitHub repo name.

    Returns:
        The matching external_link dict, or None if not found.
    """
    links = entity.get("external_links")
    if not links:
        return None

    for link in links:
        if link.get("provider") != "github":
            continue
        meta = link.get("metadata", {})
        if meta.get("owner") == owner and meta.get("repo") == repo:
            return link

    return None


# ---------------------------------------------------------------------------
# Label management
# ---------------------------------------------------------------------------


def ensure_labels(repo: str, label: str, *, dry_run: bool = False) -> None:
    """Ensure a label exists in the target GitHub repo; create if missing.

    Args:
        repo: Repository in "owner/repo" format.
        label: Label name to ensure. If empty, this is a no-op.
        dry_run: If True, pass dry_run to run_gh.
    """
    if not label:
        return

    # Check if label exists
    result = run_gh(
        [
            "gh",
            "label",
            "list",
            "--repo",
            repo,
            "--search",
            label,
            "--json",
            "name",
        ],
        dry_run=dry_run,
    )

    if result.dry_run:
        return

    # Parse response to check if exact label exists
    try:
        labels = json.loads(result.stdout) if result.stdout else []
        if any(l.get("name") == label for l in labels):
            return  # Label exists
    except (json.JSONDecodeError, TypeError):
        pass  # Treat parse failure as "not found"

    # Create the label
    run_gh(
        [
            "gh",
            "label",
            "create",
            label,
            "--repo",
            repo,
            "--description",
            f"Sudocode {label}",
            "--color",
            "0e8a16",
        ],
        dry_run=dry_run,
    )


# ---------------------------------------------------------------------------
# Atomic JSONL write
# ---------------------------------------------------------------------------


def add_external_link(
    sudocode_dir: Path | str,
    entity_id: str,
    link: dict[str, Any],
) -> None:
    """Add an external link to an entity via ``sudocode external-link add``.

    Args:
        sudocode_dir: Path to the ``.sudocode`` directory (or its parent).
        entity_id: ID of the entity (e.g. ``s-xxxx`` or ``i-xxxx``).
        link: The external link dict containing ``provider``, ``external_id``,
            ``external_url``, ``sync_direction``, ``content_hash``, and
            ``metadata``.

    Raises:
        RuntimeError: if the CLI command fails.
    """
    cmd: list[str] = [
        "external-link",
        "add",
        entity_id,
        "--provider",
        link["provider"],
        "--external-id",
        link["external_id"],
    ]
    if link.get("external_url"):
        cmd += ["--external-url", link["external_url"]]
    if link.get("sync_direction"):
        cmd += ["--sync-direction", link["sync_direction"]]
    if link.get("content_hash"):
        cmd += ["--content-hash", link["content_hash"]]
    if link.get("metadata"):
        cmd += ["--metadata", json.dumps(link["metadata"])]

    result = run_sudocode(cmd, sudocode_dir=sudocode_dir)
    if not result.success:
        raise RuntimeError(
            f"sudocode external-link add failed for {entity_id}: "
            f"{result.stderr.strip()}"
        )


def update_external_link(
    sudocode_dir: Path | str,
    entity_id: str,
    external_id: str,
    *,
    content_hash: str | None = None,
    last_synced_at: str | None = None,
    metadata: dict[str, Any] | None = None,
) -> None:
    """Update an existing external link via ``sudocode external-link update``.

    Args:
        sudocode_dir: Path to the ``.sudocode`` directory (or its parent).
        entity_id: ID of the entity (e.g. ``s-xxxx`` or ``i-xxxx``).
        external_id: The ``external_id`` of the link to update.
        content_hash: New content hash (optional).
        last_synced_at: New last-synced timestamp (optional).
        metadata: New metadata dict (optional, replaces existing).

    Raises:
        RuntimeError: if the CLI command fails.
    """
    cmd: list[str] = [
        "external-link",
        "update",
        entity_id,
        "--external-id",
        external_id,
    ]
    if content_hash is not None:
        cmd += ["--content-hash", content_hash]
    if last_synced_at is not None:
        cmd += ["--last-synced-at", last_synced_at]
    if metadata is not None:
        cmd += ["--metadata", json.dumps(metadata)]

    result = run_sudocode(cmd, sudocode_dir=sudocode_dir)
    if not result.success:
        raise RuntimeError(
            f"sudocode external-link update failed for {entity_id}: "
            f"{result.stderr.strip()}"
        )


# ---------------------------------------------------------------------------
# GitHub Issue search (duplicate prevention)
# ---------------------------------------------------------------------------


def search_github_issue(
    repo: str,
    title: str,
    *,
    dry_run: bool = False,
) -> int | None:
    """Search GitHub for an existing issue with an exact title match.

    Uses ``gh issue list --search`` to find issues that might already exist.
    Only returns a result if exactly one issue has an exact title match
    (case-sensitive). Returns None if no match, multiple matches (ambiguous),
    or on any error.

    Args:
        repo: Repository in "owner/repo" format.
        title: The exact title to search for.
        dry_run: If True, skip the search and return None.

    Returns:
        The issue number if exactly one exact match is found, None otherwise.
    """
    if dry_run:
        return None

    result = run_gh(
        [
            "gh",
            "issue",
            "list",
            "--repo",
            repo,
            "--search",
            title,
            "--json",
            "number,title",
            "--limit",
            "5",
        ],
    )

    if result.dry_run:
        return None

    if not result.success:
        print(
            f"  Warning: GitHub search failed: {result.stderr.strip()}",
            file=sys.stderr,
        )
        return None

    try:
        issues = json.loads(result.stdout)
    except (json.JSONDecodeError, TypeError):
        return None

    # Filter to exact title matches (case-sensitive)
    exact_matches = [i for i in issues if i.get("title") == title]

    if len(exact_matches) == 0:
        return None

    if len(exact_matches) > 1:
        print(
            f"  Warning: found {len(exact_matches)} issues with title "
            f"'{title}' — skipping to avoid ambiguity.",
            file=sys.stderr,
        )
        return None

    return exact_matches[0]["number"]


# ---------------------------------------------------------------------------
# GitHub Issue creation
# ---------------------------------------------------------------------------


def create_github_issue(
    *,
    repo: str,
    title: str,
    body: str,
    labels: list[str],
    dry_run: bool = False,
) -> dict[str, Any] | None:
    """Create a new GitHub Issue and return its metadata.

    Args:
        repo: Repository in "owner/repo" format.
        title: Issue title.
        body: Issue body (markdown).
        labels: Labels to apply.
        dry_run: If True, skip API calls.

    Returns:
        Dict with issue_number, issue_id, url on success; None on failure.
        In dry-run mode, returns placeholder values (0 for number/id).
    """
    # Build command
    cmd = ["gh", "issue", "create", "--repo", repo, "--title", title, "--body", body]
    for label in labels:
        cmd.extend(["--label", label])

    result = run_gh(cmd, dry_run=dry_run)

    if result.dry_run:
        return {"issue_number": 0, "issue_id": 0, "url": ""}

    if not result.success:
        print(f"  Error creating issue: {result.stderr.strip()}", file=sys.stderr)
        return None

    # Parse issue URL from stdout to get issue number
    url = result.stdout.strip()
    try:
        issue_number = int(url.rstrip("/").split("/")[-1])
    except (ValueError, IndexError):
        print(f"  Error parsing issue number from URL: {url}", file=sys.stderr)
        return None

    # Allow GitHub to propagate the issue before fetching ID
    time.sleep(1)

    # Fetch the numeric issue ID (different from number, needed for sub-issues API)
    # Retry with backoff to handle GitHub eventual consistency
    owner, repo_name = repo.split("/")
    max_id_retries = 3
    id_result = None
    for attempt in range(max_id_retries):
        if attempt > 0:
            delay = 2**attempt  # 2s, 4s
            print(
                f"  Retrying ID fetch in {delay}s (attempt {attempt + 1}/{max_id_retries})..."
            )
            time.sleep(delay)

        id_result = run_gh(
            [
                "gh",
                "api",
                f"repos/{owner}/{repo_name}/issues/{issue_number}",
                "--jq",
                ".id",
            ]
        )
        if id_result.success:
            break

    if not id_result or not id_result.success:
        # Still failed after retries — but we know the issue exists
        print(
            f"  Warning: created #{issue_number} but could not fetch ID after {max_id_retries} attempts.",
            file=sys.stderr,
        )
        return {
            "issue_number": issue_number,
            "issue_id": 0,  # sentinel: ID unknown
            "url": url,
        }

    try:
        issue_id = int(id_result.stdout.strip())
    except ValueError:
        print(
            f"  Warning: created #{issue_number} but got unparseable ID: "
            f"'{id_result.stdout.strip()}'.",
            file=sys.stderr,
        )
        return {
            "issue_number": issue_number,
            "issue_id": 0,  # sentinel: ID unknown
            "url": url,
        }

    return {
        "issue_number": issue_number,
        "issue_id": issue_id,
        "url": url,
    }


# ---------------------------------------------------------------------------
# GitHub Issue update
# ---------------------------------------------------------------------------


def update_github_issue(
    *,
    repo: str,
    issue_number: int,
    title: str,
    body: str,
    dry_run: bool = False,
) -> bool:
    """Update an existing GitHub Issue.

    Args:
        repo: Repository in "owner/repo" format.
        issue_number: Issue number to update.
        title: New title.
        body: New body (markdown).
        dry_run: If True, skip API calls.

    Returns:
        True on success, False on failure.
    """
    result = run_gh(
        [
            "gh",
            "issue",
            "edit",
            str(issue_number),
            "--repo",
            repo,
            "--title",
            title,
            "--body",
            body,
        ],
        dry_run=dry_run,
    )

    if not result.success and not result.dry_run:
        print(
            f"  Error updating issue #{issue_number}: {result.stderr.strip()}",
            file=sys.stderr,
        )
        return False

    return True


# ---------------------------------------------------------------------------
# GitHub Issue close (for status sync)
# ---------------------------------------------------------------------------


def close_github_issue(
    *,
    repo: str,
    issue_number: int,
    reason: str = "completed",
    dry_run: bool = False,
) -> bool:
    """Close a GitHub Issue to sync status with Sudocode.

    Args:
        repo: Repository in "owner/repo" format.
        issue_number: Issue number to close.
        reason: Close reason — "completed" or "not_planned".
        dry_run: If True, skip API calls.

    Returns:
        True on success (or already closed), False on failure.
    """
    cmd = [
        "gh",
        "issue",
        "close",
        str(issue_number),
        "--repo",
        repo,
    ]
    if reason == "not_planned":
        cmd.extend(["--reason", "not_planned"])
    result = run_gh(
        cmd,
        dry_run=dry_run,
    )

    if result.dry_run:
        return True

    if not result.success:
        stderr = result.stderr.strip()
        # "already closed" is not an error
        if "already closed" in stderr.lower():
            return True
        print(
            f"  Error closing issue #{issue_number}: {stderr}",
            file=sys.stderr,
        )
        return False

    return True


# ---------------------------------------------------------------------------
# Export a single entity (create/update/skip orchestrator)
# ---------------------------------------------------------------------------


def export_entity(
    *,
    entity_id: str,
    entity_type: str,
    entity_data: dict[str, Any],
    repo: str,
    owner: str,
    repo_name: str,
    ref_mapping: dict[str, int],
    spec_label: str,
    issue_label: str,
    sudocode_dir: Path,
    dry_run: bool,
    force: bool,
    summary: ExportSummary,
) -> dict[str, Any] | None:
    """Export a single entity: create, update, or skip.

    After creating or updating, syncs the GitHub Issue state to match
    the Sudocode entity status (closes the issue if status is "closed").

    Returns:
        Dict with issue_number and issue_id on success/skip, None on failure.
    """
    title = entity_data.get("title", "(untitled)")
    content = entity_data.get("content", "")
    status = entity_data.get("status", "open")

    # Rewrite references in body
    body = rewrite_references(content or "", ref_mapping)

    # Determine labels
    if entity_type == "spec":
        labels = [spec_label] if spec_label else []
    else:
        labels = [issue_label] if issue_label else []

    # Check for existing external link
    existing_link = find_external_link(entity_data, owner, repo_name)

    result_data: dict[str, Any] | None = None

    if existing_link is None:
        # Fix 5: Search GitHub for an existing issue before creating
        existing_number = search_github_issue(repo, title, dry_run=dry_run)

        if existing_number is not None:
            # Found existing issue by title search — update instead of create
            print(
                f"  Found existing GitHub Issue #{existing_number} by title search",
            )
            success = update_github_issue(
                repo=repo,
                issue_number=existing_number,
                title=title,
                body=body,
                dry_run=dry_run,
            )

            if not success:
                summary.failed += 1
                return None

            # Save the external link for future runs
            content_hash = compute_content_hash(title, content)
            now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

            link = {
                "provider": "github",
                "external_id": f"{owner}/{repo_name}#{existing_number}",
                "external_url": f"https://github.com/{owner}/{repo_name}/issues/{existing_number}",
                "sync_enabled": True,
                "sync_direction": "outbound",
                "last_synced_at": now,
                "content_hash": content_hash,
                "metadata": {
                    "github_issue_id": 0,  # ID unknown from search
                    "github_issue_number": existing_number,
                    "owner": owner,
                    "repo": repo_name,
                    "entity_type": entity_type,
                },
            }

            if not dry_run:
                add_external_link(sudocode_dir, entity_id, link)

            summary.updated += 1
            result_data = {
                "issue_number": existing_number,
                "issue_id": 0,
            }

        else:
            # No existing issue found — CREATE new GitHub Issue
            result = create_github_issue(
                repo=repo,
                title=title,
                body=body,
                labels=labels,
                dry_run=dry_run,
            )

            if result is None:
                summary.failed += 1
                return None

            # Build the external link entry
            content_hash = compute_content_hash(title, content)
            now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

            link = {
                "provider": "github",
                "external_id": f"{owner}/{repo_name}#{result['issue_number']}",
                "external_url": f"https://github.com/{owner}/{repo_name}/issues/{result['issue_number']}",
                "sync_enabled": True,
                "sync_direction": "outbound",
                "last_synced_at": now,
                "content_hash": content_hash,
                "metadata": {
                    "github_issue_id": result["issue_id"],
                    "github_issue_number": result["issue_number"],
                    "owner": owner,
                    "repo": repo_name,
                    "entity_type": entity_type,
                },
            }

            # Save external_link immediately after creation (even if issue_id=0)
            # This prevents duplicates on retry — the link exists so next run
            # will UPDATE instead of CREATE.
            if not dry_run:
                add_external_link(sudocode_dir, entity_id, link)

            # Handle partial failure: issue created but ID fetch failed
            if result["issue_id"] == 0:
                summary.failed += 1
                print(
                    f"  Warning: #{result['issue_number']} created but ID unknown. "
                    f"Re-run with --force to resolve.",
                    file=sys.stderr,
                )
                return None

            summary.created += 1
            result_data = {
                "issue_number": result["issue_number"],
                "issue_id": result["issue_id"],
            }

    else:
        # Existing link found - check if update needed
        existing_hash = existing_link.get("content_hash", "")
        current_hash = compute_content_hash(title, content)

        if not force and existing_hash == current_hash:
            # SKIP - content unchanged
            summary.skipped += 1
            result_data = {
                "issue_number": existing_link["metadata"]["github_issue_number"],
                "issue_id": existing_link["metadata"]["github_issue_id"],
            }
        else:
            # UPDATE existing GitHub Issue
            issue_number = existing_link["metadata"]["github_issue_number"]
            success = update_github_issue(
                repo=repo,
                issue_number=issue_number,
                title=title,
                body=body,
                dry_run=dry_run,
            )

            if not success:
                summary.failed += 1
                return None

            # Update the external link with new hash and timestamp
            now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

            if not dry_run:
                update_external_link(
                    sudocode_dir,
                    entity_id,
                    existing_link["external_id"],
                    content_hash=current_hash,
                    last_synced_at=now,
                )

            summary.updated += 1
            result_data = {
                "issue_number": issue_number,
                "issue_id": existing_link["metadata"]["github_issue_id"],
            }

    # Sync status: close the GitHub Issue if Sudocode status warrants it
    _CLOSE_REASON_MAP = {
        "closed": "completed",
        "wont_fix": "not_planned",
        "duplicate": "not_planned",
    }
    if result_data is not None and status in _CLOSE_REASON_MAP:
        close_ok = close_github_issue(
            repo=repo,
            issue_number=result_data["issue_number"],
            reason=_CLOSE_REASON_MAP[status],
            dry_run=dry_run,
        )
        if not close_ok:
            print(
                f"  Warning: failed to close #{result_data['issue_number']} "
                f"to match Sudocode status {status!r} for {entity_id}",
                file=sys.stderr,
            )
            summary.failed += 1
            return None

    return result_data


# ---------------------------------------------------------------------------
# Export all entities (main loop)
# ---------------------------------------------------------------------------


def export_entities(
    *,
    sorted_entities: list[tuple[str, str, dict]],
    repo: str,
    sudocode_dir: Path,
    spec_label: str,
    issue_label: str,
    dry_run: bool,
    force: bool,
    delay: float = 0.0,
) -> tuple[ExportSummary, dict[str, int], dict[str, int]]:
    """Export all entities in topological order.

    Ensures labels exist, then iterates over entities creating/updating
    GitHub Issues. Builds a ref_mapping (entity_id -> issue_number) and
    id_mapping (entity_id -> github_issue_id) as it goes so later
    entities can reference earlier ones.

    Args:
        sorted_entities: Topologically sorted list of (id, type, data).
        repo: Repository in "owner/repo" format.
        sudocode_dir: Path to .sudocode directory.
        spec_label: Label for spec entities.
        issue_label: Label for issue entities.
        dry_run: If True, skip API calls.
        force: If True, re-export all entities.

    Returns:
        Tuple of (ExportSummary, ref_mapping dict, id_mapping dict).
    """
    owner, repo_name = repo.split("/")
    summary = ExportSummary()
    ref_mapping: dict[str, int] = {}
    id_mapping: dict[str, int] = {}

    # Ensure labels exist before creating issues
    ensure_labels(repo, spec_label, dry_run=dry_run)
    ensure_labels(repo, issue_label, dry_run=dry_run)

    for entity_id, entity_type, entity_data in sorted_entities:
        print(
            f"  [{entity_type}] {entity_id}: {entity_data.get('title', '(untitled)')}"
        )

        result = export_entity(
            entity_id=entity_id,
            entity_type=entity_type,
            entity_data=entity_data,
            repo=repo,
            owner=owner,
            repo_name=repo_name,
            ref_mapping=ref_mapping,
            spec_label=spec_label,
            issue_label=issue_label,
            sudocode_dir=Path(sudocode_dir),
            dry_run=dry_run,
            force=force,
            summary=summary,
        )

        if result is not None:
            ref_mapping[entity_id] = result["issue_number"]
            id_mapping[entity_id] = result["issue_id"]

        # Inter-operation delay for API reliability
        if delay > 0 and not dry_run:
            time.sleep(delay)

    return summary, ref_mapping, id_mapping


# ---------------------------------------------------------------------------
# Structured result for gh CLI calls
# ---------------------------------------------------------------------------


@dataclass
class GhResult:
    """Structured result from a gh CLI invocation."""

    success: bool
    stdout: str
    stderr: str
    command: list[str]
    dry_run: bool = False


# ---------------------------------------------------------------------------
# Export summary
# ---------------------------------------------------------------------------


@dataclass
class ExportSummary:
    """Tracks counts of export operations for summary output."""

    created: int = 0
    updated: int = 0
    skipped: int = 0
    failed: int = 0

    def total(self) -> int:
        return self.created + self.updated + self.skipped + self.failed

    def report(self) -> str:
        return (
            f"\nExport Summary\n"
            f"  Created: {self.created}\n"
            f"  Updated: {self.updated}\n"
            f"  Skipped: {self.skipped}\n"
            f"  Failed:  {self.failed}\n"
            f"  Total:   {self.total()}"
        )


# ---------------------------------------------------------------------------
# Relationship summary
# ---------------------------------------------------------------------------


@dataclass
class RelationshipSummary:
    """Tracks counts of relationship operations for summary output."""

    sub_issues: int = 0
    dependencies: int = 0
    references: int = 0
    failed: int = 0
    skipped: int = 0

    def total(self) -> int:
        return (
            self.sub_issues
            + self.dependencies
            + self.references
            + self.failed
            + self.skipped
        )

    def report(self) -> str:
        return (
            f"\nRelationship Summary\n"
            f"  Sub-issues:    {self.sub_issues}\n"
            f"  Dependencies:  {self.dependencies}\n"
            f"  References:    {self.references}\n"
            f"  Failed:        {self.failed}\n"
            f"  Skipped:       {self.skipped}\n"
            f"  Total:         {self.total()}"
        )


# ---------------------------------------------------------------------------
# Relationship helpers
# ---------------------------------------------------------------------------


def establish_sub_issue(
    *,
    parent_number: int,
    child_github_id: int,
    owner: str,
    repo: str,
    dry_run: bool = False,
) -> bool:
    """Create a sub-issue relationship on GitHub.

    Uses the GitHub sub-issues API to make ``child_github_id`` a sub-issue
    of the issue identified by ``parent_number``.

    Args:
        parent_number: GitHub issue number of the parent.
        child_github_id: Numeric GitHub issue ID (not number) of the child.
        owner: Repository owner.
        repo: Repository name.
        dry_run: If True, skip the API call.

    Returns:
        True on success (or if the relationship already exists), False on failure.
    """
    result = run_gh(
        [
            "gh",
            "api",
            f"repos/{owner}/{repo}/issues/{parent_number}/sub_issues",
            "-X",
            "POST",
            "-H",
            "X-GitHub-Api-Version: 2026-03-10",
            "-F",
            f"sub_issue_id={child_github_id}",
        ],
        dry_run=dry_run,
    )

    if result.dry_run:
        return True

    if not result.success:
        # Idempotent: treat "already exists" as success
        if "already exists" in result.stderr.lower():
            return True
        print(
            f"  Error creating sub-issue (parent #{parent_number}, "
            f"child ID {child_github_id}): {result.stderr.strip()}",
            file=sys.stderr,
        )
        return False

    return True


def establish_dependency(
    *,
    blocked_number: int,
    blocker_github_id: int,
    owner: str,
    repo: str,
    dry_run: bool = False,
) -> bool:
    """Create a dependency relationship on GitHub.

    Uses the GitHub dependencies API to record that the issue identified
    by ``blocked_number`` is blocked by the issue identified by
    ``blocker_github_id``.

    Args:
        blocked_number: GitHub issue number of the blocked issue.
        blocker_github_id: Numeric GitHub issue ID (not number) of the blocker.
        owner: Repository owner.
        repo: Repository name.
        dry_run: If True, skip the API call.

    Returns:
        True on success (or if the dependency already exists), False on failure.
    """
    result = run_gh(
        [
            "gh",
            "api",
            f"repos/{owner}/{repo}/issues/{blocked_number}/dependencies/blocked_by",
            "-X",
            "POST",
            "-H",
            "X-GitHub-Api-Version: 2026-03-10",
            "-F",
            f"issue_id={blocker_github_id}",
        ],
        dry_run=dry_run,
    )

    if result.dry_run:
        return True

    if not result.success:
        # Idempotent: treat "already exists" as success
        if "already exists" in result.stderr.lower():
            return True
        print(
            f"  Error creating dependency (blocked #{blocked_number}, "
            f"blocker ID {blocker_github_id}): {result.stderr.strip()}",
            file=sys.stderr,
        )
        return False

    return True


def establish_reference(
    *,
    from_number: int,
    to_number: int,
    relationship_type: str,
    repo: str,
    dry_run: bool = False,
) -> bool:
    """Add a reference comment on a GitHub Issue.

    Posts a comment ``Related: #to_number`` on the issue identified by
    ``from_number``.

    Args:
        from_number: GitHub issue number to comment on.
        to_number: GitHub issue number being referenced.
        relationship_type: The relationship type (for logging).
        repo: Repository in "owner/repo" format.
        dry_run: If True, skip the API call.

    Returns:
        True on success, False on failure.
    """
    body = f"Related: #{to_number}"

    result = run_gh(
        [
            "gh",
            "issue",
            "comment",
            str(from_number),
            "--repo",
            repo,
            "--body",
            body,
        ],
        dry_run=dry_run,
    )

    if result.dry_run:
        return True

    if not result.success:
        print(
            f"  Error adding reference comment on #{from_number} -> "
            f"#{to_number}: {result.stderr.strip()}",
            file=sys.stderr,
        )
        return False

    return True


def establish_relationships(
    *,
    edges: list[tuple[str, str, str]],
    id_mapping: dict[str, int],
    ref_mapping: dict[str, int],
    owner: str,
    repo: str,
    dry_run: bool = False,
    delay: float = 0.0,
) -> RelationshipSummary:
    """Establish all relationships between exported GitHub Issues.

    Processes edges from the dependency graph and creates corresponding
    GitHub relationships:
    - ``parent-child`` and ``implements`` -> sub-issues API
    - ``blocks`` -> dependencies API (B blocked by A)
    - ``depends-on`` -> dependencies API (A blocked by B, reversed)
    - ``references``, ``related``, ``discovered-from`` -> comment

    Edges whose endpoints are not in the mappings are skipped.

    Args:
        edges: List of (from_id, to_id, edge_type) tuples.
        id_mapping: Dict mapping entity IDs to GitHub issue IDs (numeric).
        ref_mapping: Dict mapping entity IDs to GitHub issue numbers.
        owner: Repository owner.
        repo: Repository name.
        dry_run: If True, skip API calls.
        delay: Seconds to wait between API calls.

    Returns:
        RelationshipSummary with counts of operations performed.
    """
    summary = RelationshipSummary()

    for from_id, to_id, edge_type in edges:
        # Skip edges where either endpoint wasn't exported
        if from_id not in ref_mapping or from_id not in id_mapping:
            summary.skipped += 1
            continue
        if to_id not in ref_mapping or to_id not in id_mapping:
            summary.skipped += 1
            continue

        if edge_type in ("parent-child", "implements"):
            # parent-child: from_id is parent, to_id is child
            # implements: from_id is issue (child), to_id is spec (parent)
            if edge_type == "parent-child":
                parent_number = ref_mapping[from_id]
                child_github_id = id_mapping[to_id]
            else:
                # implements: spec (to_id) is parent, issue (from_id) is child
                parent_number = ref_mapping[to_id]
                child_github_id = id_mapping[from_id]

            ok = establish_sub_issue(
                parent_number=parent_number,
                child_github_id=child_github_id,
                owner=owner,
                repo=repo,
                dry_run=dry_run,
            )
            if ok:
                summary.sub_issues += 1
            else:
                summary.failed += 1

        elif edge_type == "blocks":
            # A blocks B -> B is blocked by A
            ok = establish_dependency(
                blocked_number=ref_mapping[to_id],
                blocker_github_id=id_mapping[from_id],
                owner=owner,
                repo=repo,
                dry_run=dry_run,
            )
            if ok:
                summary.dependencies += 1
            else:
                summary.failed += 1

        elif edge_type == "depends-on":
            # A depends-on B -> A is blocked by B (reverse)
            ok = establish_dependency(
                blocked_number=ref_mapping[from_id],
                blocker_github_id=id_mapping[to_id],
                owner=owner,
                repo=repo,
                dry_run=dry_run,
            )
            if ok:
                summary.dependencies += 1
            else:
                summary.failed += 1

        elif edge_type in ("references", "related", "discovered-from"):
            ok = establish_reference(
                from_number=ref_mapping[from_id],
                to_number=ref_mapping[to_id],
                relationship_type=edge_type,
                repo=f"{owner}/{repo}",
                dry_run=dry_run,
            )
            if ok:
                summary.references += 1
            else:
                summary.failed += 1

        else:
            # Unknown edge type — skip
            summary.skipped += 1

        # Inter-operation delay for API reliability
        if delay > 0 and not dry_run:
            time.sleep(delay)

    return summary


# ---------------------------------------------------------------------------
# Phase 4: Feedback export as GitHub Issue comments
# ---------------------------------------------------------------------------


@dataclass
class FeedbackSummary:
    """Tracks counts of feedback export operations."""

    exported: int = 0
    skipped: int = 0
    failed: int = 0

    def total(self) -> int:
        return self.exported + self.skipped + self.failed

    def report(self) -> str:
        return (
            f"\nFeedback Summary\n"
            f"  Exported:  {self.exported}\n"
            f"  Skipped:   {self.skipped}\n"
            f"  Failed:    {self.failed}\n"
            f"  Total:     {self.total()}"
        )


def compute_feedback_hash(content: str) -> str:
    """Compute SHA-256 hash of feedback content for deduplication.

    Args:
        content: Feedback content string.

    Returns:
        Hex-encoded SHA-256 digest.
    """
    return hashlib.sha256(content.encode("utf-8")).hexdigest()


def collect_feedback_for_export(
    entities: list[tuple[str, str, dict]],
) -> list[dict]:
    """Collect feedback entries targeting specs in the export set.

    Scans all entities (both specs and issues) for feedback entries
    whose ``to_id`` matches a spec in the export set.  Dismissed
    feedback is excluded.

    Args:
        entities: List of (entity_id, entity_type, entity_data) tuples.

    Returns:
        List of feedback dicts ready for export.
    """
    # Build set of spec IDs in the export set
    spec_ids = {eid for eid, etype, _ in entities if etype == "spec"}

    feedback_entries: list[dict] = []
    for _eid, _etype, edata in entities:
        for fb in edata.get("feedback", []):
            if fb.get("dismissed"):
                continue
            if fb.get("to_id") in spec_ids:
                feedback_entries.append(fb)

    # Sort by created_at so comments appear in chronological order
    feedback_entries.sort(key=lambda fb: fb.get("created_at", ""))

    return feedback_entries


def format_feedback_comment(
    fb: dict,
    *,
    ref_mapping: dict[str, int] | None = None,
) -> str:
    """Format a feedback entry as a GitHub Issue comment body.

    Format:
        **[Feedback from #{github_issue_number}]** {anchor context}

        {content}

    The header uses the GitHub Issue number of the issue that generated
    the feedback (``from_id``).  If the ``from_id`` is not found in
    ``ref_mapping``, falls back to ``**[{feedback_type}]**``.

    Args:
        fb: Feedback dict with ``feedback_type``, ``content``, ``from_id``,
            and optional ``anchor`` (JSON string).
        ref_mapping: Dict mapping sudocode entity IDs to GitHub issue numbers.

    Returns:
        Formatted markdown comment body.
    """
    fb_type = fb.get("feedback_type", "comment")
    content = fb.get("content", "")
    from_id = fb.get("from_id", "")
    anchor_raw = fb.get("anchor")

    # Resolve header: prefer GitHub issue number, fall back to feedback type
    github_number = None
    if ref_mapping and from_id:
        github_number = ref_mapping.get(from_id)

    if github_number is not None:
        header_tag = f"Feedback from #{github_number}"
    else:
        header_tag = fb_type

    # Build anchor context line
    anchor_context = ""
    if anchor_raw:
        try:
            anchor = json.loads(anchor_raw)
            parts: list[str] = []
            section = anchor.get("section_heading")
            if section:
                parts.append(section)
            line_num = anchor.get("line_number")
            if line_num is not None:
                parts.append(f"Line {line_num}")
            snippet = anchor.get("text_snippet")
            if snippet and not section:
                parts.append(f"`{snippet}`")
            if parts:
                anchor_context = " " + " | ".join(parts)
        except (json.JSONDecodeError, TypeError):
            pass  # Invalid anchor JSON — skip context

    header = f"**[{header_tag}]**{anchor_context}"

    return f"{header}\n\n{content}"


def post_feedback_comment(
    *,
    issue_number: int,
    body: str,
    repo: str,
    dry_run: bool = False,
) -> bool:
    """Post a feedback comment on a GitHub Issue.

    Args:
        issue_number: GitHub issue number to comment on.
        body: Comment body (markdown).
        repo: Repository in "owner/repo" format.
        dry_run: If True, skip API calls.

    Returns:
        True on success, False on failure.
    """
    result = run_gh(
        [
            "gh",
            "issue",
            "comment",
            str(issue_number),
            "--repo",
            repo,
            "--body",
            body,
        ],
        dry_run=dry_run,
    )

    if result.dry_run:
        return True

    if not result.success:
        print(
            f"  Error posting feedback comment on #{issue_number}: "
            f"{result.stderr.strip()}",
            file=sys.stderr,
        )
        return False

    return True


def export_feedback(
    *,
    entities: list[tuple[str, str, dict]],
    ref_mapping: dict[str, int],
    owner: str,
    repo: str,
    sudocode_dir: Path | None = None,
    dry_run: bool = False,
    delay: float = 0.0,
) -> FeedbackSummary:
    """Export feedback entries as comments on GitHub Issues.

    For each feedback entry targeting a spec in the export set:
    1. Compute content hash for deduplication
    2. Check if already exported via ``metadata.exported_feedback[]``
    3. Format and post as a comment on the spec's GitHub Issue
    4. Track the content hash in ``exported_feedback[]`` and write to JSONL

    Args:
        entities: List of (entity_id, entity_type, entity_data) tuples.
        ref_mapping: Dict of entity_id -> github_issue_number.
        owner: GitHub repo owner.
        repo: GitHub repo name.
        sudocode_dir: Path to .sudocode directory (for JSONL writes).
        dry_run: If True, skip API calls and JSONL writes.

    Returns:
        FeedbackSummary with counts of operations performed.
    """
    summary = FeedbackSummary()
    full_repo = f"{owner}/{repo}"

    # Collect feedback entries
    all_feedback = collect_feedback_for_export(entities)
    if not all_feedback:
        return summary

    # Build entity lookup for external_link access
    entity_lookup: dict[str, tuple[str, dict]] = {}
    for eid, etype, edata in entities:
        entity_lookup[eid] = (etype, edata)

    for fb in all_feedback:
        to_id = fb["to_id"]

        # Verify spec has a GitHub issue number
        if to_id not in ref_mapping:
            print(
                f"  Skipping feedback: {to_id} not in ref_mapping",
                file=sys.stderr,
            )
            summary.skipped += 1
            continue

        issue_number = ref_mapping[to_id]

        # Get the spec's external_link to check exported_feedback
        spec_type, spec_data = entity_lookup.get(to_id, (None, None))
        if spec_data is None:
            summary.skipped += 1
            continue

        ext_link = find_external_link(spec_data, owner, repo)
        exported_hashes: list[str] = []
        if ext_link:
            exported_hashes = ext_link.get("metadata", {}).get("exported_feedback", [])

        # Compute content hash for deduplication
        content_hash = compute_feedback_hash(fb.get("content", ""))

        if content_hash in exported_hashes:
            summary.skipped += 1
            continue

        # Format and post the comment
        body = format_feedback_comment(fb, ref_mapping=ref_mapping)
        success = post_feedback_comment(
            issue_number=issue_number,
            body=body,
            repo=full_repo,
            dry_run=dry_run,
        )

        if not success:
            summary.failed += 1
            continue

        summary.exported += 1

        # Update exported_feedback in external_link via CLI
        if not dry_run and ext_link and sudocode_dir:
            exported_hashes.append(content_hash)
            if "exported_feedback" not in ext_link.get("metadata", {}):
                ext_link.setdefault("metadata", {})["exported_feedback"] = []
            ext_link["metadata"]["exported_feedback"] = exported_hashes

            update_external_link(
                sudocode_dir,
                to_id,
                ext_link["external_id"],
                metadata=ext_link["metadata"],
            )

        # Inter-operation delay for API reliability
        if delay > 0 and not dry_run:
            time.sleep(delay)

    return summary


# ---------------------------------------------------------------------------
# gh CLI helper with rate limiting and dry-run support
# ---------------------------------------------------------------------------


def run_gh(
    command: list[str],
    *,
    dry_run: bool = False,
    max_retries: int = 5,
    base_backoff: float = 1.0,
    max_backoff: float = 60.0,
) -> GhResult:
    """Execute a gh CLI command with rate-limit retry and dry-run support.

    Args:
        command: Full command list, e.g. ["gh", "auth", "status"].
        dry_run: If True, skip execution and return a dry-run result.
        max_retries: Maximum number of retries on HTTP 429 (default 5).
        base_backoff: Base delay in seconds for exponential backoff (default 1).
        max_backoff: Maximum delay cap in seconds (default 60).

    Returns:
        GhResult with success/failure status, stdout/stderr, and command.
    """
    if dry_run:
        print(f"[dry-run] Would execute: {' '.join(command)}")
        return GhResult(
            success=True,
            stdout="",
            stderr="",
            command=command,
            dry_run=True,
        )

    # Patterns in stderr that indicate transient/retryable errors
    RETRYABLE_PATTERNS = [
        "429",
        "404",
        "500",
        "502",
        "503",
        "504",
        "connection",
        "timeout",
    ]

    attempt = 0
    while True:
        proc = subprocess.run(
            command,
            capture_output=True,
            text=True,
        )

        result = GhResult(
            success=proc.returncode == 0,
            stdout=proc.stdout,
            stderr=proc.stderr,
            command=command,
        )

        # Check for retryable errors (rate limits, transient failures, eventual consistency)
        if not result.success and attempt < max_retries:
            stderr_lower = result.stderr.lower()
            if any(p in stderr_lower for p in RETRYABLE_PATTERNS):
                delay = min(base_backoff * (2**attempt), max_backoff)
                print(
                    f"[retry] Transient error, retrying in {delay}s (attempt {attempt + 1}/{max_retries})"
                )
                time.sleep(delay)
                attempt += 1
                continue

        return result


# ---------------------------------------------------------------------------
# Startup checks
# ---------------------------------------------------------------------------


def check_gh_auth() -> None:
    """Verify that the gh CLI is installed and authenticated.

    Raises SystemExit if gh is not available or not authenticated.
    """
    result = run_gh(["gh", "auth", "status"])
    if not result.success:
        print(f"Error: gh CLI is not authenticated.", file=sys.stderr)
        print(f"  Run 'gh auth login' to authenticate.", file=sys.stderr)
        if result.stderr:
            print(f"  Details: {result.stderr.strip()}", file=sys.stderr)
        raise SystemExit(1)


def check_repo_exists(repo: str) -> None:
    """Verify that the target GitHub repository exists and is accessible.

    Args:
        repo: Repository in "owner/repo" format.

    Raises SystemExit if the repository doesn't exist or is inaccessible.
    """
    result = run_gh(["gh", "repo", "view", repo, "--json", "name", "-q", ".name"])
    if not result.success:
        print(f"Error: Repository '{repo}' not found or inaccessible.", file=sys.stderr)
        if result.stderr:
            print(f"  Details: {result.stderr.strip()}", file=sys.stderr)
        raise SystemExit(1)


def check_sudocode_dir(sudocode_dir: str) -> None:
    """Verify that the sudocode project is accessible via the CLI.

    Runs ``sudocode status`` to confirm the project is valid and the
    CLI can reach the database.

    Args:
        sudocode_dir: Path to the ``.sudocode`` directory.

    Raises SystemExit if the project is not found or the CLI fails.
    """
    result = run_sudocode(["status"], sudocode_dir=sudocode_dir)
    if not result.success:
        print(
            f"Error: Sudocode project not found or inaccessible at: {sudocode_dir}",
            file=sys.stderr,
        )
        if result.stderr:
            print(f"  Details: {result.stderr.strip()}", file=sys.stderr)
        raise SystemExit(1)


# ---------------------------------------------------------------------------
# CLI argument parser
# ---------------------------------------------------------------------------


def build_parser() -> argparse.ArgumentParser:
    """Build the argparse parser for the export CLI.

    Returns:
        Configured ArgumentParser instance.
    """
    parser = argparse.ArgumentParser(
        description="Export Sudocode specs and issues to GitHub Issues.",
        epilog=(
            "Example:\n"
            "  uv run export_to_github.py --spec-id s-2a7c --repo owner/repo\n"
            "  uv run export_to_github.py --spec-id s-2a7c --repo owner/repo --dry-run\n"
        ),
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    parser.add_argument(
        "--spec-id",
        required=True,
        help="Sudocode spec ID to export (e.g. s-2a7c)",
    )
    parser.add_argument(
        "--repo",
        required=True,
        help="Target GitHub repository in owner/repo format",
    )
    parser.add_argument(
        "--sudocode-dir",
        default=".sudocode",
        help="Path to sudocode data directory (default: .sudocode)",
    )
    parser.add_argument(
        "--spec-label",
        default="spec",
        help="Label to apply to spec GitHub Issues (default: spec)",
    )
    parser.add_argument(
        "--issue-label",
        default="",
        help="Label to apply to issue GitHub Issues (default: none)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        default=False,
        help="Print planned actions without making API calls or modifying JSONL",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        default=False,
        help="Re-export all entities regardless of content_hash",
    )
    parser.add_argument(
        "--delay",
        type=float,
        default=1.0,
        help="Delay in seconds between GitHub API operations (default: 1.0)",
    )

    return parser


# ---------------------------------------------------------------------------
# Main entry point
# ---------------------------------------------------------------------------


def main() -> None:
    """Main entry point for the export script."""
    parser = build_parser()
    args = parser.parse_args()

    # Banner
    print(f"Sudocode -> GitHub Issues Export")
    print(f"  Spec:    {args.spec_id}")
    print(f"  Repo:    {args.repo}")
    print(f"  Dir:     {args.sudocode_dir}")
    if args.dry_run:
        print(f"  Mode:    DRY RUN (no changes will be made)")
    if args.force:
        print(f"  Force:   Re-exporting all entities")
    if args.delay != 1.0:
        print(f"  Delay:   {args.delay}s between operations")
    print()

    # Startup checks (skip gh checks in dry-run mode)
    if not args.dry_run:
        check_gh_auth()
        check_repo_exists(args.repo)
    check_sudocode_dir(args.sudocode_dir)

    # Load data via sudocode CLI
    specs, issues = load_entities(args.sudocode_dir)

    # Collect graph
    try:
        entities, edges = collect_graph(args.spec_id, specs, issues)
    except KeyError as e:
        print(f"Error: {e}", file=sys.stderr)
        raise SystemExit(1)

    # Topological sort
    try:
        sorted_entities, demoted_edges = topological_sort(entities, edges)
    except CyclicDependencyError as e:
        print(f"Error: {e}", file=sys.stderr)
        raise SystemExit(1)

    if demoted_edges:
        print(
            f"Note: {len(demoted_edges)} edge(s) demoted from ordering "
            f"constraints to break cycles (relationships will still be "
            f"established on GitHub)."
        )
        print()

    print(f"Collected {len(sorted_entities)} entities, {len(edges)} edges")
    print()

    # Phase 2: Export entities (create/update GitHub Issues)
    summary, ref_mapping, id_mapping = export_entities(
        sorted_entities=sorted_entities,
        repo=args.repo,
        sudocode_dir=Path(args.sudocode_dir),
        spec_label=args.spec_label,
        issue_label=args.issue_label,
        dry_run=args.dry_run,
        force=args.force,
        delay=args.delay,
    )

    if args.dry_run:
        print("\nPlanned export order:")
        for i, (eid, etype, edata) in enumerate(sorted_entities, 1):
            title = edata.get("title", "(untitled)")
            print(f"  {i}. [{etype}] {eid}: {title}")
        print()

    print(summary.report())

    # Phase 3: Establish relationships between GitHub Issues
    owner, repo_name = args.repo.split("/")
    rel_summary = establish_relationships(
        edges=edges,
        id_mapping=id_mapping,
        ref_mapping=ref_mapping,
        owner=owner,
        repo=repo_name,
        dry_run=args.dry_run,
        delay=args.delay,
    )

    print(rel_summary.report())

    # Phase 4: Export feedback as comments on GitHub Issues
    fb_summary = export_feedback(
        entities=sorted_entities,
        ref_mapping=ref_mapping,
        owner=owner,
        repo=repo_name,
        sudocode_dir=Path(args.sudocode_dir),
        dry_run=args.dry_run,
        delay=args.delay,
    )

    print(fb_summary.report())

    # Exit with error if any failures occurred
    total_failures = summary.failed + rel_summary.failed + fb_summary.failed
    if total_failures > 0 and not args.dry_run:
        print(
            f"\nERROR: {total_failures} operation(s) failed. "
            f"GitHub Issues may be out of sync with Sudocode.",
            file=sys.stderr,
        )
        print(
            f"  Entity failures:       {summary.failed}",
            file=sys.stderr,
        )
        print(
            f"  Relationship failures: {rel_summary.failed}",
            file=sys.stderr,
        )
        print(
            f"  Feedback failures:     {fb_summary.failed}",
            file=sys.stderr,
        )
        print(
            f"\nTo fix: re-run with --force to retry failed operations.",
            file=sys.stderr,
        )
        raise SystemExit(1)


if __name__ == "__main__":
    main()
