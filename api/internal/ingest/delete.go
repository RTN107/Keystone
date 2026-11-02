package ingest

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrDocumentNotFound means the id matches no document.
var ErrDocumentNotFound = errors.New("document not found")

// Deleted says what one deletion removed, for the sentence shown to the admin.
type Deleted struct {
	DocumentID int64
	Title      string
	Passages   int
	Questions  int // logged questions whose answer cited the document
}

// DeleteDocument removes a document and everything derived from it, in one transaction:
// its passages and its access rows (the upload record in the ingestion log is read from
// those rows, so it goes with them), and every query-log row whose answer cited one of its
// passages, so nothing written from the document stays behind. Questions that did not cite
// it are untouched. Nothing is cached about a document except question vectors, which hold
// no document text.
func (s *Service) DeleteDocument(ctx context.Context, id int64) (Deleted, error) {
	d := Deleted{DocumentID: id}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.QueryRow(ctx, `SELECT title FROM documents WHERE id = $1 FOR UPDATE`, id).Scan(&d.Title); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return d, ErrDocumentNotFound
		}
		return d, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM document_chunks WHERE document_id = $1`, id).Scan(&d.Passages); err != nil {
		return d, err
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM query_logs
		WHERE cited_chunk_ids && ARRAY(SELECT c.id FROM document_chunks c WHERE c.document_id = $1)`, id)
	if err != nil {
		return d, err
	}
	d.Questions = int(tag.RowsAffected())
	// document_chunks and document_access go with it (ON DELETE CASCADE).
	if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id); err != nil {
		return d, err
	}
	return d, tx.Commit(ctx)
}
