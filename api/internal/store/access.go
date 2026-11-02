package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrUnknownDocument: the document id matches nothing.
	ErrUnknownDocument = errors.New("unknown document")
	// ErrAlreadyReadable: the team can already read the document, so there is nothing to extend.
	ErrAlreadyReadable = errors.New("team can already read the document")
	// ErrNoExtension: the team has no extension on the document to take back.
	ErrNoExtension = errors.New("no extension to revoke")
	// ErrOriginalAccess: the team was given the document when it was ingested. Only an
	// extension can be withdrawn here, so the tags a document was uploaded with stay.
	ErrOriginalAccess = errors.New("access was given at ingestion")
)

// AccessChange names what a grant or a revoke touched, for the sentence shown to the admin.
type AccessChange struct {
	DocumentID    int64
	DocumentTitle string
	TeamID        int64
	TeamName      string
}

// names loads the two names a message needs and tells the two kinds of unknown apart.
func (s *Store) accessNames(ctx context.Context, documentID, teamID int64) (AccessChange, error) {
	c := AccessChange{DocumentID: documentID, TeamID: teamID}
	if err := s.pool.QueryRow(ctx, `SELECT title FROM documents WHERE id = $1`, documentID).Scan(&c.DocumentTitle); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c, ErrUnknownDocument
		}
		return c, err
	}
	if err := s.pool.QueryRow(ctx, `SELECT name FROM teams WHERE id = $1`, teamID).Scan(&c.TeamName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c, ErrUnknownTeam
		}
		return c, err
	}
	return c, nil
}

// ExtendAccess lets a team read a document it was not ingested for. It writes one
// document_access row with source 'extension', the same row search already reads, so the
// team's next question sees it: nothing is cached about access.
func (s *Store) ExtendAccess(ctx context.Context, documentID, teamID, grantedBy int64) (AccessChange, error) {
	c, err := s.accessNames(ctx, documentID, teamID)
	if err != nil {
		return c, err
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO document_access (document_id, team_id, granted_by, source)
		VALUES ($1, $2, $3, 'extension')
		ON CONFLICT (document_id, team_id) DO NOTHING`, documentID, teamID, grantedBy)
	if err != nil {
		return c, err
	}
	if tag.RowsAffected() == 0 {
		return c, ErrAlreadyReadable
	}
	return c, nil
}

// RevokeExtension takes back an extension. It never removes access that came with the
// upload: that row has source 'ingest' and is left alone.
func (s *Store) RevokeExtension(ctx context.Context, documentID, teamID int64) (AccessChange, error) {
	c, err := s.accessNames(ctx, documentID, teamID)
	if err != nil {
		return c, err
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM document_access WHERE document_id = $1 AND team_id = $2 AND source = 'extension'`, documentID, teamID)
	if err != nil {
		return c, err
	}
	if tag.RowsAffected() == 1 {
		return c, nil
	}
	var source string
	err = s.pool.QueryRow(ctx, `SELECT source FROM document_access WHERE document_id = $1 AND team_id = $2`, documentID, teamID).Scan(&source)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return c, ErrNoExtension
	case err != nil:
		return c, err
	}
	return c, ErrOriginalAccess
}
