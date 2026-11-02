package retrieve

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned for a document that does not exist AND for one the
// team may not read. The two are deliberately indistinguishable.
var ErrNotFound = errors.New("document not found")

// readableBy is the single definition of "this team may read this document". The
// vector search, the blocked-access probe and the document viewer all build their
// WHERE clause from it, so they cannot drift apart. docID is the SQL expression
// for the document id and team is the placeholder holding the team id.
func readableBy(docID, team string) string {
	return "EXISTS (SELECT 1 FROM document_access a WHERE a.document_id = " + docID + " AND a.team_id = " + team + ")"
}

// unreadableBy is its exact negation, used by the probe over what a team cannot read.
func unreadableBy(docID, team string) string {
	return "NOT " + readableBy(docID, team)
}

// Document is what the citation viewer shows: the title and the full normalised
// text that chunk offsets index into.
type Document struct {
	ID      int64
	Title   string
	Content string
}

// Docs reads whole documents for the citation viewer.
type Docs struct{ pool *pgxpool.Pool }

func NewDocs(pool *pgxpool.Pool) *Docs { return &Docs{pool: pool} }

// Readable returns a document only if the team may read it, using the same
// predicate as the search. An unreadable document is reported as ErrNotFound,
// exactly like one that does not exist.
func (r *Docs) Readable(ctx context.Context, teamID, documentID int64) (Document, error) {
	var d Document
	err := r.pool.QueryRow(ctx,
		`SELECT d.id, d.title, d.content FROM documents d WHERE d.id = $1 AND `+readableBy("d.id", "$2"),
		documentID, teamID).Scan(&d.ID, &d.Title, &d.Content)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	return d, err
}
