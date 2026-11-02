package ingest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"keystone/internal/embed"
)

// Messages for the admin. Each says what to do next.
var (
	ErrNoTeams      = errors.New("Choose at least one team that can read this document.")
	ErrUnknownTeam  = errors.New("One of the chosen teams does not exist. Reload the page and try again.")
	ErrTitleTooLong = errors.New("Keep the title under 200 characters.")
)

const maxTitleRunes = 200

// Embedder turns text into vectors. *embed.Client satisfies it; tests use a fake.
type Embedder interface {
	Embed(ctx context.Context, texts []string, kind embed.InputType) (embed.Result, error)
}

// Service ingests documents. It is the only writer of document_chunks.
type Service struct {
	pool     *pgxpool.Pool
	embedder Embedder
}

func NewService(pool *pgxpool.Pool, e Embedder) *Service {
	return &Service{pool: pool, embedder: e}
}

// Request is one upload.
type Request struct {
	Filename   string
	Title      string // optional; taken from the first heading, then the file name
	Data       []byte
	TeamIDs    []int64
	IngestedBy int64
}

// Saved reports what was stored, in numbers the admin can trust.
type Saved struct {
	DocumentID int64
	Title      string
	Passages   int
	Tokens     int
	Teams      []string
}

// Message is the result line shown after an upload, for example "7 passages embedded".
func (s Saved) Message() string {
	if s.Passages == 1 {
		return "1 passage embedded"
	}
	return fmt.Sprintf("%d passages embedded", s.Passages)
}

// Ingest extracts, chunks and embeds a file, then saves the document, its access
// rows and its chunks in one transaction. Embedding happens first and outside the
// transaction, so a slow or failed embedding call holds no locks and, if it fails,
// leaves nothing behind.
func (s *Service) Ingest(ctx context.Context, req Request) (Saved, error) {
	teamIDs := dedupe(req.TeamIDs)
	if len(teamIDs) == 0 {
		return Saved{}, ErrNoTeams
	}

	text, err := Extract(req.Filename, req.Data)
	if err != nil {
		return Saved{}, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = DeriveTitle(req.Filename, text)
	}
	if len([]rune(title)) > maxTitleRunes {
		return Saved{}, ErrTitleTooLong
	}
	chunks := Split(text)
	if len(chunks) == 0 {
		return Saved{}, ErrEmptyDocument
	}

	teamNames, err := s.teamNames(ctx, teamIDs)
	if err != nil {
		return Saved{}, err
	}

	// The title travels with every passage, so a terse passage still says what it
	// belongs to. The stored and displayed text stays the raw chunk.
	inputs := make([]string, len(chunks))
	for i, c := range chunks {
		inputs[i] = title + "\n\n" + c.Text
	}
	res, err := s.embedder.Embed(ctx, inputs, embed.Document)
	if err != nil {
		return Saved{}, err
	}
	if len(res.Vectors) != len(chunks) {
		return Saved{}, fmt.Errorf("embedded %d passages for %d chunks", len(res.Vectors), len(chunks))
	}

	docID, err := s.save(ctx, req, title, text, teamIDs, chunks, res.Vectors)
	if err != nil {
		return Saved{}, err
	}
	return Saved{DocumentID: docID, Title: title, Passages: len(chunks), Tokens: res.Tokens, Teams: teamNames}, nil
}

const insertChunksBatch = 64

func (s *Service) save(ctx context.Context, req Request, title, text string, teamIDs []int64, chunks []Chunk, vecs [][]float32) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var docID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO documents (title, source_filename, content, ingested_by)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		title, filepath.Base(req.Filename), text, req.IngestedBy).Scan(&docID)
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO document_access (document_id, team_id, granted_by, source)
		 SELECT $1::bigint, t, $3::bigint, 'ingest' FROM unnest($2::bigint[]) AS t`,
		docID, teamIDs, req.IngestedBy); err != nil {
		return 0, err
	}

	for start := 0; start < len(chunks); start += insertChunksBatch {
		end := min(start+insertChunksBatch, len(chunks))
		n := end - start
		idx, content, emb := make([]int32, n), make([]string, n), make([]string, n)
		from, to := make([]int32, n), make([]int32, n)
		for i, c := range chunks[start:end] {
			idx[i], content[i], emb[i] = int32(c.Index), c.Text, embed.Vector(vecs[start+i])
			from[i], to[i] = int32(c.Start), int32(c.End)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO document_chunks (document_id, chunk_index, content, embedding, char_start, char_end)
			 SELECT $1::bigint, t.idx, t.content, t.emb::vector, t.s, t.e
			 FROM unnest($2::int[], $3::text[], $4::text[], $5::int[], $6::int[]) AS t(idx, content, emb, s, e)`,
			docID, idx, content, emb, from, to); err != nil {
			return 0, err
		}
	}
	return docID, tx.Commit(ctx)
}

func (s *Service) teamNames(ctx context.Context, ids []int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT name FROM teams WHERE id = ANY($1::bigint[]) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(names) != len(ids) {
		return nil, ErrUnknownTeam
	}
	return names, nil
}

func dedupe(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

var h1 = regexp.MustCompile(`(?m)^# +(.+?) *$`)

// DeriveTitle uses the first top-level heading, then the file name without its
// extension, with separators turned into spaces.
func DeriveTitle(filename, text string) string {
	if m := h1.FindStringSubmatch(text); m != nil {
		if t := strings.TrimSpace(m[1]); t != "" {
			return t
		}
	}
	base := filepath.Base(filename)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.TrimSpace(strings.NewReplacer("-", " ", "_", " ").Replace(base))
	if base == "" {
		return "Untitled document"
	}
	return base
}
