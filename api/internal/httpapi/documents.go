package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"keystone/internal/auth"
	"keystone/internal/embed"
	"keystone/internal/ingest"
)

// multipartOverhead is room for the form fields and boundaries around the file.
const multipartOverhead = 64 << 10

type documentDTO struct {
	ID         int64            `json:"id"`
	Title      string           `json:"title"`
	Filename   string           `json:"source_filename"`
	CreatedAt  time.Time        `json:"created_at"`
	IngestedBy *string          `json:"ingested_by"`
	Passages   int              `json:"passages"`
	Teams      []ingest.TeamTag `json:"teams"`
}

func toDocumentDTO(d ingest.Document) documentDTO {
	return documentDTO{ID: d.ID, Title: d.Title, Filename: d.Filename, CreatedAt: d.CreatedAt,
		IngestedBy: d.IngestedBy, Passages: d.Passages, Teams: d.Teams}
}

// ingestUnavailable answers when no embeddings key is configured.
func (s *Server) ingestUnavailable(w http.ResponseWriter) bool {
	if s.Ingest != nil {
		return false
	}
	writeError(w, http.StatusServiceUnavailable, "embeddings_unavailable",
		"This deployment has no embeddings key, so documents cannot be ingested. Set VOYAGE_API_KEY and restart.")
	return true
}

func (s *Server) adminUploadDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.ingestUnavailable(w) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, ingest.MaxUploadBytes+multipartOverhead)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "That file is over 5 MB. Split it into smaller documents and upload them one at a time.")
				return
			}
			writeError(w, http.StatusBadRequest, "bad_upload", "The upload could not be read. Choose the file again and try again.")
			return
		}
		defer r.MultipartForm.RemoveAll() //nolint:errcheck // temp files only

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "no_file", "Choose a PDF, text or Markdown file to upload.")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, ingest.MaxUploadBytes+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_upload", "The upload could not be read. Choose the file again and try again.")
			return
		}
		if len(data) > ingest.MaxUploadBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "That file is over 5 MB. Split it into smaller documents and upload them one at a time.")
			return
		}

		var teamIDs []int64
		for _, v := range r.MultipartForm.Value["team_id"] {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "unknown_team", ingest.ErrUnknownTeam.Error())
				return
			}
			teamIDs = append(teamIDs, id)
		}

		p, _ := auth.PrincipalFrom(r.Context())
		saved, err := s.Ingest.Ingest(r.Context(), ingest.Request{
			Filename:   header.Filename,
			Title:      r.FormValue("title"),
			Data:       data,
			TeamIDs:    teamIDs,
			IngestedBy: p.ID,
		})
		if err != nil {
			s.writeIngestError(w, err)
			return
		}
		s.Log.Info("document ingested", "document_id", saved.DocumentID, "passages", saved.Passages, "tokens", saved.Tokens)
		writeJSON(w, http.StatusCreated, map[string]any{
			"document_id": saved.DocumentID,
			"title":       saved.Title,
			"passages":    saved.Passages,
			"tokens":      saved.Tokens,
			"teams":       saved.Teams,
			"message":     saved.Message(),
		})
	}
}

// writeIngestError turns a failure into a sentence the admin can act on. The
// ingest errors already are such sentences; embedding failures are explained
// here, and every failure says that nothing was saved.
func (s *Server) writeIngestError(w http.ResponseWriter, err error) {
	var apiErr *embed.APIError
	var dimErr *embed.DimensionError
	var unreachable *embed.UnreachableError
	switch {
	case errors.Is(err, ingest.ErrUnsupportedType):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", err.Error())
	case errors.Is(err, ingest.ErrEmptyDocument), errors.Is(err, ingest.ErrNoPDFText), errors.Is(err, ingest.ErrUnreadablePDF):
		writeError(w, http.StatusUnprocessableEntity, "unreadable_file", err.Error())
	case errors.Is(err, ingest.ErrNoTeams):
		writeError(w, http.StatusBadRequest, "no_teams", err.Error())
	case errors.Is(err, ingest.ErrUnknownTeam):
		writeError(w, http.StatusBadRequest, "unknown_team", err.Error())
	case errors.Is(err, ingest.ErrTitleTooLong):
		writeError(w, http.StatusBadRequest, "title_too_long", err.Error())
	case errors.As(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests:
		s.Log.Warn("embeddings rate limited", "error", err)
		writeError(w, http.StatusServiceUnavailable, "embeddings_busy",
			"The embeddings service is rate limited, which happens on a free plan. Nothing was saved. Wait a minute, then upload the file again.")
	case errors.As(err, &unreachable):
		s.Log.Error("embeddings unreachable", "error", err)
		writeError(w, http.StatusBadGateway, "embeddings_unreachable",
			"Keystone could not reach the embeddings service. Nothing was saved. Check VOYAGE_BASE_URL and the server's network access, then try again.")
	case errors.As(err, &apiErr), errors.As(err, &dimErr):
		s.Log.Error("embeddings failed", "error", err)
		writeError(w, http.StatusBadGateway, "embeddings_failed",
			"Keystone could not get embeddings for this document. Nothing was saved. Check the embeddings key and try again.")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusServiceUnavailable, "cancelled", "The upload was interrupted. Nothing was saved. Try again.")
	default:
		s.Log.Error("ingest failed", "error", err)
		writeError(w, http.StatusInternalServerError, "ingest_failed", "Keystone could not save this document. Nothing was saved. Try again in a moment.")
	}
}

func (s *Server) adminListDocuments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.ingestUnavailable(w) {
			return
		}
		docs, err := s.Ingest.Documents(r.Context())
		if err != nil {
			s.Log.Error("list documents", "error", err)
			internalError(w)
			return
		}
		out := make([]documentDTO, len(docs))
		for i, d := range docs {
			out[i] = toDocumentDTO(d)
		}
		writeJSON(w, http.StatusOK, map[string]any{"documents": out})
	}
}

func (s *Server) adminIngestionLog() http.HandlerFunc {
	type entry struct {
		DocumentID int64     `json:"document_id"`
		Title      string    `json:"title"`
		Filename   string    `json:"source_filename"`
		At         time.Time `json:"at"`
		By         *string   `json:"by"`
		Passages   int       `json:"passages"`
		Teams      []string  `json:"teams"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if s.ingestUnavailable(w) {
			return
		}
		log, err := s.Ingest.IngestionLog(r.Context())
		if err != nil {
			s.Log.Error("ingestion log", "error", err)
			internalError(w)
			return
		}
		out := make([]entry, len(log))
		for i, e := range log {
			out[i] = entry{e.DocumentID, e.Title, e.Filename, e.At, e.By, e.Passages, e.Teams}
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": out})
	}
}

// adminDeleteDocument removes a document, its passages, its upload record and every logged
// question whose answer cited it. It cannot be undone, so the screen asks first and the
// message says exactly what went.
func (s *Server) adminDeleteDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.ingestUnavailable(w) {
			return
		}
		id, ok := pathID(r, "id")
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "That document does not exist. Reload the documents list and try again.")
			return
		}
		d, err := s.Ingest.DeleteDocument(r.Context(), id)
		switch {
		case errors.Is(err, ingest.ErrDocumentNotFound):
			writeError(w, http.StatusNotFound, "not_found", "That document does not exist. Reload the documents list and try again.")
			return
		case err != nil:
			s.Log.Error("delete document", "error", err)
			internalError(w)
			return
		}
		p, _ := auth.PrincipalFrom(r.Context())
		s.Log.Info("document deleted", "document_id", d.DocumentID, "passages", d.Passages, "questions_removed", d.Questions, "by", p.ID)
		writeJSON(w, http.StatusOK, map[string]any{
			"document_id": d.DocumentID, "title": d.Title, "passages": d.Passages, "questions_removed": d.Questions,
			"message": fmt.Sprintf("Deleted %s: %s, its upload record and %s that cited it. Keystone can no longer use it.",
				d.Title, plural(d.Passages, "passage"), plural(d.Questions, "logged question")),
		})
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
