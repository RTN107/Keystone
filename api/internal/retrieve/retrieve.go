// Package retrieve is the only place a question meets the document chunks.
//
// Everything the model may ever see comes out of Search, whose single SQL
// statement carries the team constraint in the same statement as the nearest
// neighbour ordering. Permitted values can only be made here (their fields are
// unexported), and the answer package accepts nothing else. A second, separate
// probe looks at what the team CANNOT read, and returns a score and team names
// only: no text, no title, nothing that could be put in a prompt.
package retrieve

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"keystone/internal/embed"
)

const (
	// SearchDepth is how many nearest permitted passages the search returns.
	SearchDepth = 6
	// MaxPassages is how many of them, at most, go to the model. Only passages at
	// or above the relevance floor are used.
	MaxPassages = 4
)

// Embedder turns a question into a vector. *embed.Client satisfies it.
type Embedder interface {
	Embed(ctx context.Context, texts []string, kind embed.InputType) (embed.Result, error)
}

// Permitted is a passage the asking team is allowed to read. Its fields are
// unexported on purpose: other packages cannot build one, so a function that
// takes []Permitted can only be handed passages that came out of Search.
type Permitted struct {
	chunkID    int64
	documentID int64
	title      string
	index      int
	text       string
	charStart  int
	charEnd    int
	score      float64
}

func (p Permitted) ChunkID() int64        { return p.chunkID }
func (p Permitted) DocumentID() int64     { return p.documentID }
func (p Permitted) DocumentTitle() string { return p.title }
func (p Permitted) ChunkIndex() int       { return p.index }
func (p Permitted) Text() string          { return p.text }
func (p Permitted) CharStart() int        { return p.charStart }
func (p Permitted) CharEnd() int          { return p.charEnd }
func (p Permitted) Score() float64        { return p.score }

// Result is the outcome of one question. Passages is filled only when the
// outcome is Answered. HoldingTeams is filled only when it is Blocked, and holds
// team names and nothing else about the document.
type Result struct {
	Outcome       Outcome
	Passages      []Permitted
	BestPermitted float64
	BestForbidden float64
	HoldingTeams  []string
	// QueryCached reports that the question's vector came from the cache, so no
	// embedding call was made.
	QueryCached bool
}

type Retriever struct {
	pool     *pgxpool.Pool
	embedder Embedder
	cache    *vectorCache
	th       Thresholds
}

// New builds a retriever. Pass DefaultThresholds unless a test needs others.
func New(pool *pgxpool.Pool, e Embedder, th Thresholds) *Retriever {
	return &Retriever{pool: pool, embedder: e, cache: newVectorCache(DefaultCacheEntries), th: th}
}

func (r *Retriever) Thresholds() Thresholds { return r.th }

// Retrieve embeds the question (or reuses the cached vector), searches what the
// team may read, probes what it may not, and decides. teamID must come from a
// fresh database lookup of the signed-in employee, never from the request.
func (r *Retriever) Retrieve(ctx context.Context, teamID int64, question string) (Result, error) {
	vec, cached, err := r.queryVector(ctx, question)
	if err != nil {
		return Result{}, err
	}
	lit := embed.Vector(vec)

	permitted, err := r.search(ctx, teamID, lit)
	if err != nil {
		return Result{}, err
	}
	forbiddenScore, holders, err := r.probe(ctx, teamID, lit)
	if err != nil {
		return Result{}, err
	}

	res := Result{BestPermitted: NoScore, BestForbidden: forbiddenScore, QueryCached: cached}
	if len(permitted) > 0 {
		res.BestPermitted = permitted[0].score
	}
	res.Outcome = Decide(r.th, res.BestPermitted, res.BestForbidden)
	switch res.Outcome {
	case Answered:
		for _, p := range permitted {
			if p.score < r.th.Floor || len(res.Passages) == MaxPassages {
				break
			}
			res.Passages = append(res.Passages, p)
		}
	case Blocked:
		res.HoldingTeams = holders
	}
	return res, nil
}

func (r *Retriever) queryVector(ctx context.Context, question string) ([]float32, bool, error) {
	key := normalizeQuestion(question)
	if v, ok := r.cache.get(key); ok {
		return v, true, nil
	}
	out, err := r.embedder.Embed(ctx, []string{question}, embed.Query)
	if err != nil {
		return nil, false, err
	}
	if len(out.Vectors) != 1 {
		return nil, false, fmt.Errorf("embedded 1 question and got %d vectors", len(out.Vectors))
	}
	r.cache.put(key, out.Vectors[0])
	return out.Vectors[0], false, nil
}

// searchSQL is the one statement that reads chunk text for answering. The team
// constraint is in the WHERE clause of the same statement as the ordering by
// distance, so a passage the team cannot read is never a candidate: it cannot be
// ranked, limited, returned or prompted. Exact scan, no index.
var searchSQL = `
	SELECT c.id, c.document_id, d.title, c.chunk_index, c.content, c.char_start, c.char_end,
	       1 - (c.embedding <=> $1::vector) AS score
	FROM document_chunks c
	JOIN documents d ON d.id = c.document_id
	WHERE ` + readableBy("c.document_id", "$2") + `
	ORDER BY c.embedding <=> $1::vector
	LIMIT $3`

func (r *Retriever) search(ctx context.Context, teamID int64, vec string) ([]Permitted, error) {
	rows, err := r.pool.Query(ctx, searchSQL, vec, teamID, SearchDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Permitted
	for rows.Next() {
		var p Permitted
		if err := rows.Scan(&p.chunkID, &p.documentID, &p.title, &p.index, &p.text, &p.charStart, &p.charEnd, &p.score); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// probeSQL looks at what the team CANNOT read, and selects a similarity score and
// team names only. No chunk text and no document title are selected, so nothing
// from a forbidden document can reach the caller, let alone a prompt.
var probeSQL = `
	WITH best AS (
		SELECT c.document_id, 1 - (c.embedding <=> $1::vector) AS score
		FROM document_chunks c
		WHERE ` + unreadableBy("c.document_id", "$2") + `
		ORDER BY c.embedding <=> $1::vector
		LIMIT 1
	)
	SELECT best.score,
	       COALESCE((SELECT array_agg(t.name ORDER BY t.id)
	                 FROM document_access a JOIN teams t ON t.id = a.team_id
	                 WHERE a.document_id = best.document_id), '{}')
	FROM best`

func (r *Retriever) probe(ctx context.Context, teamID int64, vec string) (float64, []string, error) {
	rows, err := r.pool.Query(ctx, probeSQL, vec, teamID)
	if err != nil {
		return NoScore, nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return NoScore, nil, rows.Err()
	}
	var score float64
	var holders []string
	if err := rows.Scan(&score, &holders); err != nil {
		return NoScore, nil, err
	}
	return score, holders, rows.Err()
}

// Describe is a one-line summary for logs, free of any document content.
func (r Result) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "outcome=%s permitted=%.3f forbidden=%.3f passages=%d", r.Outcome, r.BestPermitted, r.BestForbidden, len(r.Passages))
	return b.String()
}

func vectorLiteral(v []float32) string { return embed.Vector(v) }

// Warm embeds the given questions in one request and remembers their vectors, so asking one
// of them later needs no embeddings call. The cache holds vectors only, so warming never
// stores an answer, a score or a permission. A failure is returned for the caller to log and
// is harmless: the question is simply embedded when it is first asked.
func (r *Retriever) Warm(ctx context.Context, questions []string) error {
	res, err := r.embedder.Embed(ctx, questions, embed.Query)
	if err != nil {
		return err
	}
	for i, q := range questions {
		r.cache.put(normalizeQuestion(q), res.Vectors[i])
	}
	return nil
}
