package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"polling-system/internal/domain/poll"
	"polling-system/internal/domain/user"
	"polling-system/internal/domain/vote"
	jwtpkg "polling-system/internal/platform/jwt"
	"polling-system/internal/worker"
)

type Handler struct {
	userSvc *user.Service
	pollSvc *poll.Service
	voteSvc *vote.Service
	jwtMgr  *jwtpkg.Manager
	agg     worker.Aggregator
	voteCh  chan<- worker.VoteEvent
	db      *sql.DB
}

func NewRouter(
	userSvc *user.Service,
	pollSvc *poll.Service,
	voteSvc *vote.Service,
	jwtMgr *jwtpkg.Manager,
	agg worker.Aggregator,
	voteCh chan<- worker.VoteEvent,
	db *sql.DB,
	trustedProxies []netip.Prefix,
) http.Handler {
	h := &Handler{
		userSvc: userSvc,
		pollSvc: pollSvc,
		voteSvc: voteSvc,
		jwtMgr:  jwtMgr,
		agg:     agg,
		voteCh:  voteCh,
		db:      db,
	}

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(RealIPMiddleware(trustedProxies))
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))
	r.Use(RequestLogger)
	r.Use(CORSMiddleware)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/ready", h.handleReady)
	r.Get("/swagger/*", httpSwagger.WrapHandler)
	r.Get("/metrics", promhttp.Handler().ServeHTTP)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", h.handleRegister)
		r.Post("/auth/login", h.handleLogin)

		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(jwtMgr, userSvc))

			r.Get("/polls", h.handleListPolls)
			r.Get("/polls/{id}", h.handleGetPoll)
			r.With(RateLimitVotes(rate.Every(time.Minute/10), 3)).Post("/polls/{id}/vote", h.handleVote)
			r.Get("/polls/{id}/results", h.handlePollResults)

			r.Group(func(r chi.Router) {
				r.Use(RequireRole("admin"))
				r.Post("/polls", h.handleCreatePoll)
				r.Patch("/polls/{id}", h.handleUpdatePoll)
				r.Patch("/polls/{id}/status", h.handleUpdatePollStatus)
				r.Delete("/polls/{id}", h.handleDeletePoll)
				r.Get("/users", h.handleListUsers)
				r.Patch("/users/{id}/role", h.handleUpdateUserRole)
				r.Patch("/users/{id}/deactivate", h.handleDeactivateUser)
			})
		})
	})

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parseIDParam(r *http.Request, name string) (int64, error) {
	idStr := chi.URLParam(r, name)
	return strconv.ParseInt(idStr, 10, 64)
}

func parseTimePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	return &t
}

func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "db_unavailable",
			"message": "database not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "db_unavailable",
			"message": "database not ready",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
