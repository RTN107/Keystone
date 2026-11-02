package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"keystone/internal/answer"
	"keystone/internal/auth"
	"keystone/internal/embed"
	"keystone/internal/retrieve"
	"keystone/internal/store"
)

const maxQuestionRunes = 500

// Ask rate limits. Every ask costs an embeddings request and a model call, so the
// public deployment cannot be left open to a loop. Three layers, each in memory:
//   - per client address: Config.AsksPerIP (default 6) in 10 minutes, whatever account is
//     used, so several shared logins do not multiply it;
//   - across the deployment, an hour: 300, a brake for bursts;
//   - across the deployment, a day: Config.DailyAskCap (default 150), after which Keystone
//     rests until the oldest counted question is a day old. No model or embeddings call is
//     made once it is reached: the check runs before anything is spent.
const (
	askPerClientWindow = 10 * time.Minute
	askGlobalMax       = 300
	askGlobalWindow    = time.Hour
	askDailyWindow     = 24 * time.Hour
)

// sse writes server-sent events and flushes each one as it is written, so tokens
// reach the browser as the model produces them.
type sse struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func newSSE(w http.ResponseWriter) *sse {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("X-Accel-Buffering", "no") // tell proxies not to hold the stream back
	w.WriteHeader(http.StatusOK)
	s := &sse{w: w, rc: http.NewResponseController(w)}
	_ = s.rc.Flush()
	return s
}

func (s *sse) event(name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, b)
	_ = s.rc.Flush()
}

// Citation is what the client needs to draw one source in the margin and open it
// in the viewer. Passage is the cited chunk's own text, which the asker's team is
// allowed to read, since it came out of the team-scoped search.
type citationDTO struct {
	N             int    `json:"n"`
	ChunkID       int64  `json:"chunk_id"`
	DocumentID    int64  `json:"document_id"`
	DocumentTitle string `json:"document_title"`
	Passage       string `json:"passage"`
	CharStart     int    `json:"char_start"`
	CharEnd       int    `json:"char_end"`
}

func (s *Server) ask() http.HandlerFunc {
	type req struct {
		Question string `json:"question"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// The team comes from the database row Authenticate just loaded for this
		// request. Nothing in the body or the token can change it.
		p, _ := auth.PrincipalFrom(r.Context())
		if p.TeamID == nil || p.TeamName == nil {
			writeError(w, http.StatusForbidden, "admins_cannot_ask",
				"Admin accounts manage Keystone and cannot ask questions. Sign in as an employee to ask.")
			return
		}
		if s.Retriever == nil {
			writeError(w, http.StatusServiceUnavailable, "embeddings_unavailable",
				"This deployment has no embeddings key, so questions cannot be searched. Set VOYAGE_API_KEY and restart.")
			return
		}
		if s.Model == nil {
			writeError(w, http.StatusServiceUnavailable, "answers_unavailable",
				"This deployment has no answering key, so questions cannot be answered. Set KEYSTONE_ANTHROPIC_KEY and restart.")
			return
		}

		var in req
		if !decodeJSON(w, r, &in) {
			return
		}
		question := strings.TrimSpace(in.Question)
		switch n := utf8.RuneCountInString(question); {
		case n == 0:
			writeError(w, http.StatusBadRequest, "empty_question", "Type a question first.")
			return
		case n > maxQuestionRunes:
			writeError(w, http.StatusBadRequest, "question_too_long", fmt.Sprintf("Keep the question under %d characters.", maxQuestionRunes))
			return
		}

		// The cheapest refusal goes first. The daily cap is checked before the per-address
		// window so a resting deployment never spends a slot of anyone's allowance.
		if ok, wait := s.DailyAsks.Peek("all"); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "resting",
				"Keystone is resting for today. It has answered as many questions as it is set up to answer in a day. Try again tomorrow.")
			return
		}
		if ok, wait := s.ClientAsks.Allow(clientIP(r, s.Config.ProxyHops)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf(
				"You have asked several questions in a short time. Keystone allows %d every 10 minutes for each visitor. Wait about %s, then ask again.",
				orPositive(s.Config.AsksPerIP, 6), humanWait(wait)))
			return
		}
		if ok, _ := s.GlobalAsks.Allow("all"); !ok {
			w.Header().Set("Retry-After", "300")
			writeError(w, http.StatusTooManyRequests, "busy",
				"Keystone is getting a lot of questions right now. Try again in a few minutes.")
			return
		}
		s.DailyAsks.Allow("all") // counted only now that the question will be answered

		stream := newSSE(w)
		ctx := r.Context()

		// If the embeddings service is rate limited, say so on the stream before
		// the answer, instead of leaving the person with silence.
		embCtx := embed.WithRateLimitNotice(ctx, func(rl embed.RateLimitWait) {
			msg := answer.Waiting(rl.Attempt, rl.MaxAttempts, rl.Wait)
			if rl.Queued {
				msg = answer.Queued
			}
			stream.event("waiting", map[string]any{
				"reason":           "embeddings_rate_limited",
				"attempt":          rl.Attempt,
				"max_attempts":     rl.MaxAttempts,
				"retry_in_seconds": int(rl.Wait.Round(time.Second) / time.Second),
				"message":          msg,
			})
		})

		res, err := s.Retriever.Retrieve(embCtx, *p.TeamID, question)
		if err != nil {
			s.retrievalFailed(stream, ctx, err)
			return
		}
		s.Log.Info("ask decided", "employee", p.Number, "outcome", string(res.Outcome),
			"client", clientIP(r, s.Config.ProxyHops), "xff_entries", strings.Count(r.Header.Get("X-Forwarded-For"), ",")+1,
			"best_permitted", round3(res.BestPermitted), "best_forbidden", round3(res.BestForbidden),
			"passages", len(res.Passages), "query_cached", res.QueryCached)

		stream.event("start", map[string]any{"outcome": res.Outcome})

		switch res.Outcome {
		case retrieve.Blocked:
			// A static template. The model is not called, and the holding team is the
			// only thing about the forbidden document that is used.
			msg := answer.RefusalBlocked(*p.TeamName, res.HoldingTeams)
			s.record(ctx, p, question, store.QueryLog{Outcome: "blocked", AnswerText: msg})
			stream.event("done", map[string]any{
				"outcome": res.Outcome, "message": msg,
				"holding_teams": res.HoldingTeams, "your_team": *p.TeamName,
				"citations": []citationDTO{},
			})

		case retrieve.NoMatch:
			s.record(ctx, p, question, store.QueryLog{Outcome: "no_match", AnswerText: answer.NoMatch})
			stream.event("done", map[string]any{"outcome": res.Outcome, "message": answer.NoMatch, "citations": []citationDTO{}})

		default:
			s.answerQuestion(stream, ctx, p, question, res)
		}
	}
}

func (s *Server) answerQuestion(stream *sse, ctx context.Context, p auth.Principal, question string, res retrieve.Result) {
	started := false
	out, err := answer.Generate(ctx, s.Model, question, res.Passages, func(text string) {
		started = true
		stream.event("token", map[string]string{"text": text})
	})
	if err != nil {
		if ctx.Err() != nil {
			return // the person went away
		}
		s.Log.Error("answer failed", "error", err)
		msg := answer.AnswerFailed
		if started {
			msg = answer.AnswerFailedPartway
		}
		stream.event("error", map[string]any{"code": "answer_failed", "message": msg, "partial": started})
		return
	}
	if out.Dropped > 0 {
		s.Log.Warn("removed citation markers that named no supplied passage", "count", out.Dropped)
	}
	if out.Truncated {
		s.Log.Warn("answer hit the length limit")
	}

	cites := make([]citationDTO, 0, len(out.Citations))
	chunkIDs := make([]int64, 0, len(out.Citations))
	for _, c := range out.Citations {
		ps := c.Passage
		cites = append(cites, citationDTO{N: c.N, ChunkID: ps.ChunkID(), DocumentID: ps.DocumentID(), DocumentTitle: ps.DocumentTitle(),
			Passage: ps.Text(), CharStart: ps.CharStart(), CharEnd: ps.CharEnd()})
		chunkIDs = append(chunkIDs, ps.ChunkID())
	}
	s.record(ctx, p, question, store.QueryLog{Outcome: "answered", AnswerText: out.Text, CitedChunkIDs: chunkIDs})
	// Citations attach here, once the prose is complete and every marker has been
	// checked against the passages that were supplied.
	stream.event("done", map[string]any{"outcome": res.Outcome, "answer": out.Text, "citations": cites, "truncated": out.Truncated})
}

func (s *Server) retrievalFailed(stream *sse, ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	var apiErr *embed.APIError
	var unreachable *embed.UnreachableError
	switch {
	case errors.As(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests:
		s.Log.Warn("embeddings stayed rate limited", "error", err)
		stream.event("error", map[string]any{"code": "embeddings_busy", "message": answer.EmbeddingsBusy})
	case errors.As(err, &unreachable):
		s.Log.Error("embeddings unreachable", "error", err)
		stream.event("error", map[string]any{"code": "embeddings_unreachable", "message": answer.EmbeddingsUnreachable})
	default:
		s.Log.Error("retrieval failed", "error", err)
		stream.event("error", map[string]any{"code": "internal", "message": "Something went wrong on our side. Try again in a moment."})
	}
}

// record writes the ask to query_logs. It is not tied to the request's lifetime,
// so an answer is logged even if the browser closed just as it finished. A failure
// to log is reported to the operator and never to the person asking.
func (s *Server) record(ctx context.Context, p auth.Principal, question string, q store.QueryLog) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	q.EmployeeID, q.TeamID, q.Question = p.ID, *p.TeamID, question
	if _, err := s.Store.RecordQuery(ctx, q); err != nil {
		s.Log.Error("could not record the question", "error", err)
	}
}

func humanWait(d time.Duration) string {
	switch mins := int((d + 59*time.Second) / time.Minute); {
	case d < time.Minute:
		return "a minute"
	case mins == 1:
		return "a minute"
	default:
		return fmt.Sprintf("%d minutes", mins)
	}
}

func round3(f float64) float64 { return float64(int(f*1000+0.5*sign(f))) / 1000 }

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// viewDocument serves the citation viewer. It reads through the same access
// predicate as the search, so a team that cannot have a passage quoted at it
// cannot open the document either. An unreadable document and a missing one are
// the same 404.
func (s *Server) viewDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFrom(r.Context())
		if p.TeamID == nil {
			writeError(w, http.StatusForbidden, "forbidden", "Your account does not have access to this.")
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "That document is not available to your team.")
			return
		}
		doc, err := s.Docs.Readable(r.Context(), *p.TeamID, id)
		if errors.Is(err, retrieve.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "That document is not available to your team.")
			return
		}
		if err != nil {
			s.Log.Error("read document", "error", err)
			internalError(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": doc.ID, "title": doc.Title, "content": doc.Content})
	}
}

func (s *Server) adminQueryLogs() http.HandlerFunc {
	type entry struct {
		ID             int64     `json:"id"`
		At             time.Time `json:"at"`
		EmployeeNumber string    `json:"employee_number"`
		EmployeeName   string    `json:"employee_name"`
		TeamID         int64     `json:"team_id"`
		Team           string    `json:"team"`
		Question       string    `json:"question"`
		Outcome        string    `json:"outcome"`
		Answer         *string   `json:"answer"`
		CitedChunkIDs  []int64   `json:"cited_chunk_ids"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		f := store.QueryLogFilter{EmployeeNumber: r.URL.Query().Get("employee")}
		if v := r.URL.Query().Get("team_id"); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "bad_filter", "The team filter is not a valid team.")
				return
			}
			f.TeamID = id
		}
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				writeError(w, http.StatusBadRequest, "bad_filter", "The limit must be a positive number.")
				return
			}
			f.Limit = n
		}
		rows, err := s.Store.QueryLogs(r.Context(), f)
		if err != nil {
			s.Log.Error("query logs", "error", err)
			internalError(w)
			return
		}
		out := make([]entry, len(rows))
		for i, q := range rows {
			out[i] = entry{q.ID, q.At, q.EmployeeNumber, q.EmployeeName, q.TeamID, q.TeamName, q.Question, q.Outcome, q.AnswerText, q.CitedChunkIDs}
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": out})
	}
}
