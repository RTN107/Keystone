// Package httpapi wires routes, middleware and handlers.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"keystone/internal/answer"
	"keystone/internal/auth"
	"keystone/internal/config"
	"keystone/internal/ingest"
	"keystone/internal/ratelimit"
	"keystone/internal/retrieve"
	"keystone/internal/store"
)

type Deps struct {
	Store   *store.Store
	Issuer  *auth.Issuer
	Limiter *auth.LoginLimiter
	Config  config.Config
	// Ingest is nil when no embeddings key is configured.
	Ingest *ingest.Service
	// Retriever is nil without an embeddings key, Model nil without an answering key.
	Retriever *retrieve.Retriever
	Model     answer.Model
	// Docs serves the citation viewer through the same access predicate as search.
	Docs *retrieve.Docs
	// ClientAsks and GlobalAsks limit how often Ask may be used. Left nil, they take
	// the defaults in ask.go.
	ClientAsks *ratelimit.Window
	GlobalAsks *ratelimit.Window
	// DailyAsks is the deployment-wide cap per 24 hours. Left nil it is built from the config.
	DailyAsks *ratelimit.Window
	Log       *slog.Logger
}

type Server struct{ Deps }

// access says who may call a route. Public routes need no token.
type access int

const (
	public access = iota
	signedIn
	adminOnly
)

type route struct {
	pattern string // "METHOD /path"
	access  access
	handler func(*Server) http.HandlerFunc
}

// routes is the single list of endpoints. Tests walk it to prove every write
// route is refused on a read-only deployment.
func routes() []route {
	return []route{
		{"GET /api/health", public, (*Server).health},
		{"POST /api/setup", public, (*Server).setup},
		{"POST /api/auth/login", public, (*Server).login},
		{"GET /api/me", signedIn, (*Server).me},
		{"GET /api/admin/teams", adminOnly, (*Server).adminTeams},
		{"POST /api/admin/employees", adminOnly, (*Server).adminCreateEmployee},
		{"POST /api/admin/documents", adminOnly, (*Server).adminUploadDocument},
		{"GET /api/admin/documents", adminOnly, (*Server).adminListDocuments},
		{"DELETE /api/admin/documents/{id}", adminOnly, (*Server).adminDeleteDocument},
		{"POST /api/admin/documents/{id}/access", adminOnly, (*Server).adminExtendAccess},
		{"DELETE /api/admin/documents/{id}/access/{team_id}", adminOnly, (*Server).adminRevokeAccess},
		{"GET /api/admin/ingestion-log", adminOnly, (*Server).adminIngestionLog},
		{"GET /api/admin/query-logs", adminOnly, (*Server).adminQueryLogs},
		{"POST /api/ask", signedIn, (*Server).ask},
		{"GET /api/documents/{id}", signedIn, (*Server).viewDocument},
	}
}

func New(d Deps) http.Handler {
	if d.ClientAsks == nil {
		d.ClientAsks = ratelimit.New(orPositive(d.Config.AsksPerIP, 6), askPerClientWindow)
	}
	if d.DailyAsks == nil {
		d.DailyAsks = ratelimit.New(orPositive(d.Config.DailyAskCap, 150), askDailyWindow)
	}
	if d.GlobalAsks == nil {
		d.GlobalAsks = ratelimit.New(askGlobalMax, askGlobalWindow)
	}
	s := &Server{d}
	mux := http.NewServeMux()

	authenticate := auth.Authenticate(d.Issuer, s.lookupPrincipal, writeError)
	requireAdmin := auth.RequireRole(auth.RoleAdmin, writeError)

	for _, rt := range routes() {
		var h http.Handler = rt.handler(s)
		switch rt.access {
		case adminOnly:
			h = authenticate(requireAdmin(h))
		case signedIn:
			h = authenticate(h)
		}
		mux.Handle(rt.pattern, h)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "That endpoint does not exist.")
	})

	// Outermost first: log, then CORS, then the read-only guard, then routing.
	var h http.Handler = mux
	h = readOnly(d.Config.ReadOnly)(h)
	h = cors(d.Config.WebOrigin)(h)
	h = requestLog(d.Log)(h)
	return h
}

func (s *Server) lookupPrincipal(ctx context.Context, id int64) (auth.Principal, error) {
	e, err := s.Store.EmployeeByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return auth.Principal{}, auth.ErrUnknownEmployee
	}
	if err != nil {
		return auth.Principal{}, err
	}
	return auth.Principal{ID: e.ID, Number: e.Number, Name: e.Name, Role: e.Role, TeamID: e.TeamID, TeamName: e.TeamName}, nil
}

// orPositive is n, or def when n is not above zero (a Config built by hand in a test).
func orPositive(n, def int) int {
	if n > 0 {
		return n
	}
	return def
}
