package httpapi

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// readOnlyAllowed is the complete list of non-safe requests a read-only
// deployment still accepts. Everything else that could change data is refused by
// default, so a write route added later is protected without anyone remembering
// to protect it.
var readOnlyAllowed = map[string]bool{
	"POST /api/auth/login": true,
	"POST /api/ask":        true,
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func readOnly(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) || readOnlyAllowed[r.Method+" "+r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusForbidden, "read_only",
				"This deployment is read-only. Documents, access and accounts cannot be changed here.")
		})
	}
}

// cors allows exactly one origin, the web app's. Anything else gets no CORS headers.
func cors(allowed string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			if r.Header.Get("Origin") == allowed {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", allowed)
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the real writer (flushing, deadlines).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// requestLog records method, path, status and duration. It never logs headers,
// bodies or query strings, so credentials and questions stay out of the logs.
func requestLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				if v := recover(); v != nil {
					log.Error("panic in handler", "path", r.URL.Path, "panic", v)
					if rec.status == http.StatusOK {
						internalError(rec)
					}
				}
				log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// clientIP is the caller's address. Behind trusted proxies each one appends the address
// it received the request from to X-Forwarded-For, so the entries on the LEFT were written
// by the client and can be forged, and the entry `hops` places from the right is the one a
// trusted proxy wrote. With hops of 1 that is the last entry. Without the header the socket
// address is used.
func clientIP(r *http.Request, hops int) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		i := len(parts) - max(hops, 1)
		if i < 0 {
			i = 0
		}
		return strings.TrimSpace(parts[i])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
