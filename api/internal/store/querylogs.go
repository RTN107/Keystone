package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// DefaultMaxQueryLogRows is the most rows query_logs keeps. Recording a question
// prunes the oldest beyond it, so the table cannot be flooded into the database's
// size limit. A row is at most a few kilobytes, so the cap is a few tens of
// megabytes at worst.
const DefaultMaxQueryLogRows = 5000

// QueryLog is one recorded question.
type QueryLog struct {
	EmployeeID    int64
	TeamID        int64
	Question      string
	Outcome       string // answered, blocked or no_match
	AnswerText    string
	CitedChunkIDs []int64
}

// RecordQuery stores a question and its outcome, then prunes the oldest rows
// beyond the cap. It runs in read-only mode too: a viewer seeing their own
// question land in the log is part of the demo, and the cap and the ask rate limit
// are what keep it from being flooded.
func (s *Store) RecordQuery(ctx context.Context, q QueryLog) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	cited := q.CitedChunkIDs
	if cited == nil {
		cited = []int64{}
	}
	var answer *string
	if q.AnswerText != "" {
		answer = &q.AnswerText
	}
	var id int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO query_logs (employee_id, team_id, question, outcome, answer_text, cited_chunk_ids)
		 VALUES ($1, $2, $3, $4, $5, $6::bigint[]) RETURNING id`,
		q.EmployeeID, q.TeamID, q.Question, q.Outcome, answer, cited).Scan(&id); err != nil {
		return 0, err
	}
	// Everything older than the newest maxRows rows goes.
	if _, err := tx.Exec(ctx,
		`DELETE FROM query_logs
		 WHERE id <= (SELECT id FROM query_logs ORDER BY id DESC OFFSET $1 LIMIT 1)`, s.maxQueryLogRows()); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (s *Store) maxQueryLogRows() int {
	if s.maxLogRows > 0 {
		return s.maxLogRows
	}
	return DefaultMaxQueryLogRows
}

// WithMaxQueryLogRows sets the retention cap, for tests.
func (s *Store) WithMaxQueryLogRows(n int) *Store {
	s.maxLogRows = n
	return s
}

// QueryLogFilter narrows the admin's view of the log. Zero values mean no filter.
type QueryLogFilter struct {
	TeamID         int64
	EmployeeNumber string
	Limit          int
}

// LoggedQuery is a row of the log with the names the admin screen needs.
type LoggedQuery struct {
	ID             int64
	At             time.Time
	EmployeeNumber string
	EmployeeName   string
	TeamID         int64
	TeamName       string
	Question       string
	Outcome        string
	AnswerText     *string
	CitedChunkIDs  []int64
}

// QueryLogs lists recorded questions, newest first.
func (s *Store) QueryLogs(ctx context.Context, f QueryLogFilter) ([]LoggedQuery, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT l.id, l.created_at, e.employee_number, e.name, l.team_id, t.name,
		       l.question, l.outcome, l.answer_text, l.cited_chunk_ids
		FROM query_logs l
		JOIN employees e ON e.id = l.employee_id
		JOIN teams t ON t.id = l.team_id
		WHERE ($1::bigint = 0 OR l.team_id = $1)
		  AND ($2::text = '' OR e.employee_number = $2)
		ORDER BY l.id DESC
		LIMIT $3`, f.TeamID, strings.ToUpper(strings.TrimSpace(f.EmployeeNumber)), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (LoggedQuery, error) {
		var q LoggedQuery
		err := r.Scan(&q.ID, &q.At, &q.EmployeeNumber, &q.EmployeeName, &q.TeamID, &q.TeamName,
			&q.Question, &q.Outcome, &q.AnswerText, &q.CitedChunkIDs)
		if q.CitedChunkIDs == nil {
			q.CitedChunkIDs = []int64{}
		}
		return q, err
	})
}
