package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"keystone/internal/answer"
	"keystone/internal/auth"
	"keystone/internal/config"
	"keystone/internal/db"
	"keystone/internal/embed"
	"keystone/internal/httpapi"
	"keystone/internal/ingest"
	"keystone/internal/retrieve"
	"keystone/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("keystone stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Migrations run first, on the direct host, so a fresh deploy needs no manual
	// database step.
	if err := db.Migrate(ctx, cfg.MigrationURL(), log); err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	st := store.New(pool)
	seeds := make([]store.TeamSeed, len(cfg.Teams))
	for i, t := range cfg.Teams {
		seeds[i] = store.TeamSeed{Name: t.Name, Prefix: t.Prefix}
	}
	if err := st.EnsureTeams(ctx, seeds); err != nil {
		return err
	}
	if n, err := st.CountTeams(ctx); err == nil && n == 0 {
		log.Warn("no teams exist yet: set KEYSTONE_TEAMS so documents and people can belong to a team")
	}

	var (
		ingestSvc *ingest.Service
		retriever *retrieve.Retriever
		model     answer.Model
	)
	if cfg.EmbeddingsEnabled() {
		embedder := embed.New(cfg.VoyageBaseURL, cfg.VoyageAPIKey, embed.Dimension)
		if cfg.VoyageModel != "" {
			embedder = embedder.WithModel(cfg.VoyageModel)
		}
		th := retrieve.DefaultThresholds
		if cfg.RelevanceFloor > 0 {
			th.Floor = cfg.RelevanceFloor
		}
		if cfg.BlockMargin > 0 {
			th.Margin = cfg.BlockMargin
		}
		log.Info("retrieval thresholds", "relevance_floor", th.Floor, "block_margin", th.Margin)
		ingestSvc = ingest.NewService(pool, embedder)
		retriever = retrieve.New(pool, embedder, th)
	}
	if cfg.AnswersEnabled() {
		model = answer.NewClient(cfg.AnthropicAPIKey, cfg.AnthropicModel)
	}

	handler := httpapi.New(httpapi.Deps{
		Store:   st,
		Issuer:  auth.NewIssuer(cfg.JWTSecret),
		Limiter: auth.NewLoginLimiter(),
		Config:  cfg,
		Ingest:  ingestSvc,

		Retriever: retriever,
		Model:     model,
		Docs:      retrieve.NewDocs(pool),
		Log:       log,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second, // uploads
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: answers stream for as long as the model takes.
	}

	// Questions named in KEYSTONE_WARM_QUESTIONS are embedded once at boot, in one request, so
	// they never wait on the embeddings service when first asked. It runs beside the server,
	// never in front of it: the health check does not wait for it, and a failure only means
	// those questions are embedded when first asked. Nothing is warmed unless you list some.
	if retriever != nil && len(cfg.WarmQuestions) > 0 {
		go func() {
			wctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()
			if err := retriever.Warm(wctx, cfg.WarmQuestions); err != nil {
				log.Warn("could not warm the query cache", "error", err)
				return
			}
			log.Info("query cache warm", "questions", len(cfg.WarmQuestions))
		}()
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("keystone listening", "addr", srv.Addr, "read_only", cfg.ReadOnly,
			"embeddings", cfg.EmbeddingsEnabled(), "answers", cfg.AnswersEnabled())
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
	return nil
}
