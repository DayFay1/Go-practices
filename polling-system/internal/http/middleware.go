package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"polling-system/internal/domain/user"
	"polling-system/internal/metrics"
	"polling-system/internal/platform/apperr"
	jwtpkg "polling-system/internal/platform/jwt"
)

type ctxKey string

const (
	ctxKeyUserID ctxKey = "user_id"
	ctxKeyRole   ctxKey = "role"
)

var slogLogger = slog.Default()

func SetLogger(l *slog.Logger) {
	if l != nil {
		slogLogger = l
	}
}

type UserLookup interface {
	GetByID(ctx context.Context, id int64) (*user.User, error)
}

func AuthMiddleware(jm *jwtpkg.Manager, users UserLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" {
				errorResponse(w, apperr.Unauthorized("missing_token", "missing authorization header", nil))
				return
			}

			parts := strings.SplitN(h, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				errorResponse(w, apperr.Unauthorized("invalid_token", "invalid authorization header", nil))
				return
			}

			claims, err := jm.Parse(parts[1])
			if err != nil {
				errorResponse(w, apperr.Unauthorized("invalid_token", "invalid token", err))
				return
			}

			if users != nil {
				u, err := users.GetByID(r.Context(), claims.UserID)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						errorResponse(w, apperr.Unauthorized("invalid_token", "invalid token", err))
						return
					}
					errorResponse(w, apperr.Internal("internal_error", "failed to authorize", err))
					return
				}
				if !u.IsActive {
					errorResponse(w, user.ErrInactiveUser)
					return
				}
				claims.UserID = u.ID
				claims.Role = u.Role
			}

			ctx := context.WithValue(r.Context(), ctxKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ctxKeyRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctxRole, ok := r.Context().Value(ctxKeyRole).(string)
			if !ok || ctxRole != role {
				errorResponse(w, apperr.Forbidden("forbidden", "insufficient permissions", nil))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func userIDFromCtx(r *http.Request) int64 {
	if v := r.Context().Value(ctxKeyUserID); v != nil {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RateLimitVotes(r rate.Limit, burst int) func(http.Handler) http.Handler {
	limiter := newIPRateLimiter(r, burst, 10*time.Minute)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !limiter.allow(ip) {
				writeJSON(w, http.StatusTooManyRequests, map[string]string{
					"error":   "rate_limited",
					"message": "too many requests",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(rw, r)

		status := rw.Status()
		if status == 0 {
			status = http.StatusOK
		}
		route := r.URL.Path
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}

		metrics.IncRequest(r.Method, route, status)

		slogLogger.Info("request",
			"method", r.Method,
			"path", route,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	lastSeen map[string]time.Time
	limit    rate.Limit
	burst    int
	entryTTL time.Duration
}

func newIPRateLimiter(limit rate.Limit, burst int, entryTTL time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		lastSeen: make(map[string]time.Time),
		limit:    limit,
		burst:    burst,
		entryTTL: entryTTL,
	}
}

func (l *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for key, ts := range l.lastSeen {
		if now.Sub(ts) > l.entryTTL {
			delete(l.limiters, key)
			delete(l.lastSeen, key)
		}
	}

	if limiter, ok := l.limiters[ip]; ok {
		l.lastSeen[ip] = now
		return limiter
	}
	limiter := rate.NewLimiter(l.limit, l.burst)
	l.limiters[ip] = limiter
	l.lastSeen[ip] = now
	return limiter
}

func (l *ipRateLimiter) allow(ip string) bool {
	limiter := l.getLimiter(ip)
	return limiter.Allow()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func RealIPMiddleware(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	if len(trustedProxies) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			remoteIP := remoteAddrIP(r.RemoteAddr)
			if !remoteIP.IsValid() || !isTrustedProxy(remoteIP, trustedProxies) {
				next.ServeHTTP(w, r)
				return
			}

			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				if ip := parseFirstForwardedFor(fwd); ip.IsValid() {
					r.RemoteAddr = ip.String()
				}
			} else if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
				if ip := parseIP(realIP); ip.IsValid() {
					r.RemoteAddr = ip.String()
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func remoteAddrIP(remoteAddr string) netip.Addr {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return parseIP(host)
	}
	return parseIP(remoteAddr)
}

func parseFirstForwardedFor(xff string) netip.Addr {
	for _, part := range strings.Split(xff, ",") {
		ip := parseIP(part)
		if ip.IsValid() {
			return ip
		}
	}
	return netip.Addr{}
}

func parseIP(s string) netip.Addr {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") && strings.Contains(s, "]") {
		s = strings.TrimPrefix(s, "[")
		s = strings.TrimSuffix(s, "]")
	}
	ip, err := netip.ParseAddr(s)
	if err == nil {
		return ip
	}
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return netip.Addr{}
	}
	ip, err = netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return ip
}

func isTrustedProxy(ip netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, p := range trustedProxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
