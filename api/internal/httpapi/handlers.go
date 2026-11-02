package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"keystone/internal/auth"
	"keystone/internal/store"
)

type teamDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type employeeDTO struct {
	ID     int64    `json:"id"`
	Number string   `json:"employee_number"`
	Name   string   `json:"name"`
	Role   string   `json:"role"`
	Team   *teamDTO `json:"team"`
}

func toEmployeeDTO(e store.Employee) employeeDTO {
	d := employeeDTO{ID: e.ID, Number: e.Number, Name: e.Name, Role: e.Role}
	if e.TeamID != nil && e.TeamName != nil {
		d.Team = &teamDTO{ID: *e.TeamID, Name: *e.TeamName}
	}
	return d
}

type sessionDTO struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expires_at"`
	Employee  employeeDTO `json:"employee"`
}

func (s *Server) newSession(e store.Employee) (sessionDTO, error) {
	token, exp, err := s.Issuer.Issue(e.ID)
	return sessionDTO{Token: token, ExpiresAt: exp, Employee: toEmployeeDTO(e)}, err
}

// health is public. It tells the web app what this deployment can do, so the
// interface can be honest about it instead of showing controls that fail.
func (s *Server) health() http.HandlerFunc {
	type capabilities struct {
		Embeddings bool `json:"embeddings"`
		Answers    bool `json:"answers"`
	}
	type body struct {
		Status       string       `json:"status"`
		ReadOnly     bool         `json:"read_only"`
		NeedsSetup   bool         `json:"needs_setup"`
		Capabilities capabilities `json:"capabilities"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		needs, err := s.Store.NeedsSetup(r.Context())
		if err != nil {
			s.Log.Error("health check failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, body{Status: "degraded"})
			return
		}
		writeJSON(w, http.StatusOK, body{
			Status:       "ok",
			ReadOnly:     s.Config.ReadOnly,
			NeedsSetup:   needs,
			Capabilities: capabilities{Embeddings: s.Config.EmbeddingsEnabled(), Answers: s.Config.AnswersEnabled()},
		})
	}
}

// setup creates the first admin. It works exactly once, while no employee exists.
func (s *Server) setup() http.HandlerFunc {
	type req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if !decodeJSON(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			writeError(w, http.StatusBadRequest, "invalid_name", "Enter a name for the admin account.")
			return
		}
		if err := auth.ValidatePassword(in.Password); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
			return
		}
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			internalError(w)
			return
		}
		e, err := s.Store.CreateFirstAdmin(r.Context(), in.Name, hash)
		if errors.Is(err, store.ErrAlreadySetUp) {
			writeError(w, http.StatusConflict, "already_set_up", "Keystone already has an admin. Sign in with your employee number.")
			return
		}
		if err != nil {
			s.Log.Error("create first admin", "error", err)
			internalError(w)
			return
		}
		sess, err := s.newSession(e)
		if err != nil {
			internalError(w)
			return
		}
		writeJSON(w, http.StatusCreated, sess)
	}
}

func (s *Server) login() http.HandlerFunc {
	type req struct {
		Number   string `json:"employee_number"`
		Password string `json:"password"`
	}
	const wrong = "That employee number and password do not match. Check both and try again."
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if !decodeJSON(w, r, &in) {
			return
		}
		number := store.NormalizeNumber(in.Number)
		key := clientIP(r, s.Config.ProxyHops) + "|" + number
		if !s.Limiter.Allow(key) {
			writeError(w, http.StatusTooManyRequests, "too_many_attempts", "Too many sign-in attempts. Wait a few minutes, then try again.")
			return
		}
		e, err := s.Store.EmployeeByNumber(r.Context(), number)
		switch {
		case errors.Is(err, store.ErrNotFound):
			auth.BurnPasswordCheck(in.Password)
			s.Limiter.Fail(key)
			writeError(w, http.StatusUnauthorized, "invalid_credentials", wrong)
			return
		case err != nil:
			s.Log.Error("login lookup", "error", err)
			internalError(w)
			return
		}
		if !auth.CheckPassword(e.PasswordHash, in.Password) {
			s.Limiter.Fail(key)
			writeError(w, http.StatusUnauthorized, "invalid_credentials", wrong)
			return
		}
		s.Limiter.Reset(key)
		sess, err := s.newSession(e)
		if err != nil {
			internalError(w)
			return
		}
		writeJSON(w, http.StatusOK, sess)
	}
}

func (s *Server) me() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFrom(r.Context())
		e, err := s.Store.EmployeeByID(r.Context(), p.ID)
		if err != nil {
			internalError(w)
			return
		}
		writeJSON(w, http.StatusOK, toEmployeeDTO(e))
	}
}

func (s *Server) adminTeams() http.HandlerFunc {
	type teamWithMembers struct {
		teamDTO
		NumberPrefix string        `json:"number_prefix"`
		Members      []employeeDTO `json:"members"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		teams, err := s.Store.TeamsWithMembers(r.Context())
		if err != nil {
			s.Log.Error("list teams", "error", err)
			internalError(w)
			return
		}
		out := make([]teamWithMembers, 0, len(teams))
		for _, t := range teams {
			item := teamWithMembers{teamDTO: teamDTO{ID: t.ID, Name: t.Name}, NumberPrefix: t.NumberPrefix, Members: []employeeDTO{}}
			for _, m := range t.Members {
				item.Members = append(item.Members, toEmployeeDTO(m))
			}
			out = append(out, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{"teams": out})
	}
}

func (s *Server) adminCreateEmployee() http.HandlerFunc {
	type req struct {
		Name     string `json:"name"`
		TeamID   int64  `json:"team_id"`
		Number   string `json:"employee_number"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if !decodeJSON(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			writeError(w, http.StatusBadRequest, "invalid_name", "Enter the employee's name.")
			return
		}
		if err := auth.ValidatePassword(in.Password); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
			return
		}
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			internalError(w)
			return
		}
		e, err := s.Store.CreateEmployee(r.Context(), store.NewEmployee{Name: in.Name, TeamID: in.TeamID, Number: in.Number, PasswordHash: hash})
		switch {
		case errors.Is(err, store.ErrUnknownTeam):
			writeError(w, http.StatusBadRequest, "unknown_team", "Choose one of the three teams.")
		case errors.Is(err, store.ErrInvalidNumber):
			writeError(w, http.StatusBadRequest, "invalid_number", "Employee numbers look like DEL-001: a team prefix, a hyphen, then at least three digits.")
		case errors.Is(err, store.ErrNumberTaken):
			writeError(w, http.StatusConflict, "number_taken", "That employee number is already in use. Choose another, or leave it blank to assign the next one.")
		case err != nil:
			s.Log.Error("create employee", "error", err)
			internalError(w)
		default:
			writeJSON(w, http.StatusCreated, toEmployeeDTO(e))
		}
	}
}
