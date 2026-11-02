package ingest

import (
	"context"
	"time"
)

// TeamTag is a team that can read a document.
type TeamTag struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"` // "ingest" or "extension"
}

// Document is a row of the ingested-documents list.
type Document struct {
	ID         int64
	Title      string
	Filename   string
	CreatedAt  time.Time
	IngestedBy *string // employee number
	Passages   int
	Teams      []TeamTag
}

// Documents lists every document, newest first, with the teams that can read it.
// Passage counts come from document_chunks, which only this package may read.
func (s *Service) Documents(ctx context.Context) ([]Document, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.title, d.source_filename, d.created_at, e.employee_number,
		       (SELECT count(*) FROM document_chunks c WHERE c.document_id = d.id)
		FROM documents d
		LEFT JOIN employees e ON e.id = d.ingested_by
		ORDER BY d.created_at DESC, d.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := []Document{}
	byID := map[int64]int{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.Title, &d.Filename, &d.CreatedAt, &d.IngestedBy, &d.Passages); err != nil {
			return nil, err
		}
		d.Teams = []TeamTag{}
		byID[d.ID] = len(docs)
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	access, err := s.pool.Query(ctx, `
		SELECT a.document_id, t.id, t.name, a.source
		FROM document_access a JOIN teams t ON t.id = a.team_id
		ORDER BY a.document_id, t.id`)
	if err != nil {
		return nil, err
	}
	defer access.Close()
	for access.Next() {
		var docID int64
		var tag TeamTag
		if err := access.Scan(&docID, &tag.ID, &tag.Name, &tag.Source); err != nil {
			return nil, err
		}
		if i, ok := byID[docID]; ok {
			docs[i].Teams = append(docs[i].Teams, tag)
		}
	}
	return docs, access.Err()
}

// LogEntry is one ingestion: what went in, when, by whom, and to which teams. It
// is read from the access rows written at upload (source 'ingest'), so a later
// extension to another team never rewrites history.
type LogEntry struct {
	DocumentID int64
	Title      string
	Filename   string
	At         time.Time
	By         *string // employee number
	Passages   int
	Teams      []string
}

// IngestionLog lists ingestions, newest first.
func (s *Service) IngestionLog(ctx context.Context) ([]LogEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.title, d.source_filename, a.granted_at, e.employee_number, t.name,
		       (SELECT count(*) FROM document_chunks c WHERE c.document_id = d.id)
		FROM document_access a
		JOIN documents d ON d.id = a.document_id
		JOIN teams t ON t.id = a.team_id
		LEFT JOIN employees e ON e.id = a.granted_by
		WHERE a.source = 'ingest'
		ORDER BY a.granted_at DESC, d.id DESC, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []LogEntry{}
	last := map[int64]int{}
	for rows.Next() {
		var (
			e    LogEntry
			team string
		)
		if err := rows.Scan(&e.DocumentID, &e.Title, &e.Filename, &e.At, &e.By, &team, &e.Passages); err != nil {
			return nil, err
		}
		if i, ok := last[e.DocumentID]; ok {
			out[i].Teams = append(out[i].Teams, team)
			continue
		}
		e.Teams = []string{team}
		last[e.DocumentID] = len(out)
		out = append(out, e)
	}
	return out, rows.Err()
}
