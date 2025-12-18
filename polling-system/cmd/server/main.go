package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "polling-system/docs"
	"polling-system/internal/config"
	"polling-system/internal/domain/poll"
	"polling-system/internal/domain/user"
	"polling-system/internal/domain/vote"
	api "polling-system/internal/http"
	"polling-system/internal/metrics"
	"polling-system/internal/platform/database"
	jwtpkg "polling-system/internal/platform/jwt"
	"polling-system/internal/repository/postgres"
	"polling-system/internal/worker"
)

// @title           Polling System API
// @version         1.0
// @description     Simple polling platform with JWT auth
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	api.SetLogger(logger)

	cfg := config.Load()
	metrics.Register()

	db, err := database.NewPostgres(cfg.DB_DSN)
	if err != nil {
		logger.Error("db connect error", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	userRepo := postgres.NewUserRepo(db)
	pollRepo := postgres.NewPollRepo(db)
	voteRepo := postgres.NewVoteRepo(db)

	userSvc := user.NewService(userRepo)
	pollSvc := poll.NewService(pollRepo)
	voteSvc := vote.NewService(voteRepo)

	jwtMgr := jwtpkg.NewManager(cfg.JWTSecret, cfg.JWTIssuer)

	voteCh := make(chan worker.VoteEvent, 100)
	statsWorker := worker.NewStatsWorker(voteCh, voteRepo, logger)

	router := api.NewRouter(userSvc, pollSvc, voteSvc, jwtMgr, voteRepo, voteCh, db, cfg.TrustedProxies)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	workerDone := make(chan struct{})

	go func() {
		statsWorker.Run(workerCtx)
		close(workerDone)
	}()

	go func() {
		logger.Info("server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down...", "signal", ctx.Err())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	close(voteCh)

	select {
	case <-workerDone:
	case <-shutdownCtx.Done():
		logger.Warn("worker shutdown timed out")
		workerCancel()
	}

	logger.Info("server stopped")
}
