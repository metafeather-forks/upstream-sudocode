package projects

import (
	"context"
	"time"
)

// --- Response types ---

type ReadyResponse struct {
	ReadyIssues []IssueResponse `json:"ready_issues"`
}

type ProjectStatusResponse struct {
	ReadyIssues      []IssueResponse `json:"ready_issues"`
	InProgressIssues []IssueResponse `json:"in_progress_issues"`
	BlockedIssues    []IssueResponse `json:"blocked_issues"`
	SpecCount        int             `json:"spec_count"`
	IssueCount       int             `json:"issue_count"`
}

// --- Endpoints ---

// Ready returns issues that have no blocking dependencies (ready to work on).
// An issue is "ready" when it is open and no other open issue blocks it.
//
//encore:api auth method=GET path=/ready
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

// ProjectStatus returns an overview of the project state.
//
//encore:api auth method=GET path=/status
func ProjectStatus(ctx context.Context) (*ProjectStatusResponse, error) {
	pid := projectID(ctx)

	// Get ready issues
	readyResp, err := Ready(ctx)
	if err != nil {
		return nil, err
	}

	// Get in-progress issues
	ipRows, err := db.Query(ctx, `
		SELECT id, title, status, content, priority, assignee, archived, parent_id, closed_at, created_at, updated_at
		FROM issues WHERE project_id = $1 AND status = 'in_progress' AND archived = false
		ORDER BY priority ASC, created_at ASC
	`, pid)
	if err != nil {
		return nil, err
	}
	defer ipRows.Close()
	inProgress, err := scanIssues(ctx, pid, ipRows)
	if err != nil {
		return nil, err
	}

	// Get blocked issues
	bRows, err := db.Query(ctx, `
		SELECT id, title, status, content, priority, assignee, archived, parent_id, closed_at, created_at, updated_at
		FROM issues WHERE project_id = $1 AND status = 'blocked' AND archived = false
		ORDER BY priority ASC, created_at ASC
	`, pid)
	if err != nil {
		return nil, err
	}
	defer bRows.Close()
	blocked, err := scanIssues(ctx, pid, bRows)
	if err != nil {
		return nil, err
	}

	// Counts
	var specCount, issueCount int
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM specs WHERE project_id = $1 AND archived = false`, pid).Scan(&specCount)
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM issues WHERE project_id = $1 AND archived = false`, pid).Scan(&issueCount)

	return &ProjectStatusResponse{
		ReadyIssues:      readyResp.ReadyIssues,
		InProgressIssues: inProgress,
		BlockedIssues:    blocked,
		SpecCount:        specCount,
		IssueCount:       issueCount,
	}, nil
}

// scanIssues is a helper to scan issue rows into response structs.
type issueScanner interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}

func scanIssues(ctx context.Context, pid string, rows issueScanner) ([]IssueResponse, error) {
	var issues []IssueResponse
	for rows.Next() {
		var i IssueResponse
		var archived bool
		var parentID, assignee *string
		var closedAt *time.Time
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&i.ID, &i.Title, &i.Status, &i.Content, &i.Priority, &assignee, &archived, &parentID, &closedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		i.Archived = archived
		i.ParentID = parentID
		i.Assignee = assignee
		if closedAt != nil {
			s := closedAt.Format(time.RFC3339)
			i.ClosedAt = &s
		}
		i.CreatedAt = createdAt.Format(time.RFC3339)
		i.UpdatedAt = updatedAt.Format(time.RFC3339)
		var err error
		i.Tags, err = tagsForEntity(ctx, pid, "issue", i.ID)
		if err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	if issues == nil {
		issues = []IssueResponse{}
	}
	return issues, rows.Err()
}
