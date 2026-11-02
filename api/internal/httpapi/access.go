package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"keystone/internal/auth"
	"keystone/internal/store"
)

// pathID reads a positive integer path value, so a bad id is a plain 404 and never reaches SQL.
func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

// writeAccessError says what to do next, in the voice of the other admin messages.
func (s *Server) writeAccessError(w http.ResponseWriter, c store.AccessChange, err error) {
	switch {
	case errors.Is(err, store.ErrUnknownDocument):
		writeError(w, http.StatusNotFound, "not_found", "That document does not exist. Reload the documents list and try again.")
	case errors.Is(err, store.ErrUnknownTeam):
		writeError(w, http.StatusBadRequest, "unknown_team", "Choose one of the three teams.")
	case errors.Is(err, store.ErrAlreadyReadable):
		writeError(w, http.StatusConflict, "already_readable", c.TeamName+" can already read "+c.DocumentTitle+". Nothing was changed.")
	case errors.Is(err, store.ErrNoExtension):
		writeError(w, http.StatusNotFound, "no_extension", c.TeamName+" has no extra access to "+c.DocumentTitle+". Nothing was changed.")
	case errors.Is(err, store.ErrOriginalAccess):
		writeError(w, http.StatusConflict, "original_access", c.TeamName+" was given "+c.DocumentTitle+" when it was uploaded, so that access is not taken back here. Nothing was changed.")
	default:
		s.Log.Error("change access", "error", err)
		internalError(w)
	}
}

// adminExtendAccess lets another team read a document. The next question that team asks is
// answered from it, and a question it had been refused is answered when it is asked again.
func (s *Server) adminExtendAccess() http.HandlerFunc {
	type req struct {
		TeamID int64 `json:"team_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		docID, ok := pathID(r, "id")
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "That document does not exist. Reload the documents list and try again.")
			return
		}
		var in req
		if !decodeJSON(w, r, &in) {
			return
		}
		p, _ := auth.PrincipalFrom(r.Context())
		c, err := s.Store.ExtendAccess(r.Context(), docID, in.TeamID, p.ID)
		if err != nil {
			s.writeAccessError(w, c, err)
			return
		}
		s.Log.Info("access extended", "document_id", c.DocumentID, "team_id", c.TeamID, "by", p.ID)
		writeJSON(w, http.StatusCreated, map[string]any{
			"document_id": c.DocumentID,
			"title":       c.DocumentTitle,
			"team":        teamDTO{ID: c.TeamID, Name: c.TeamName},
			"message":     c.TeamName + " can now read " + c.DocumentTitle + ". Keystone will use it the next time " + c.TeamName + " asks a question.",
		})
	}
}

// adminRevokeAccess takes back an extension. Access a document was uploaded with is not removable here.
func (s *Server) adminRevokeAccess() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID, ok1 := pathID(r, "id")
		teamID, ok2 := pathID(r, "team_id")
		if !ok1 || !ok2 {
			writeError(w, http.StatusNotFound, "not_found", "That document does not exist. Reload the documents list and try again.")
			return
		}
		c, err := s.Store.RevokeExtension(r.Context(), docID, teamID)
		if err != nil {
			s.writeAccessError(w, c, err)
			return
		}
		p, _ := auth.PrincipalFrom(r.Context())
		s.Log.Info("access revoked", "document_id", c.DocumentID, "team_id", c.TeamID, "by", p.ID)
		writeJSON(w, http.StatusOK, map[string]any{
			"document_id": c.DocumentID,
			"title":       c.DocumentTitle,
			"team":        teamDTO{ID: c.TeamID, Name: c.TeamName},
			"message":     c.TeamName + " can no longer read " + c.DocumentTitle + ". Keystone will stop using it from the next question " + c.TeamName + " asks.",
		})
	}
}
