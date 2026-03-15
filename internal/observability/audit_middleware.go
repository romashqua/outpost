package observability

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/romashqua/outpost/internal/auth"
)

// statusWriter wraps http.ResponseWriter to capture the response status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// AuditMiddleware returns chi-compatible middleware that automatically records
// state-changing HTTP requests (POST, PUT, PATCH, DELETE) to the audit log.
// The user_id is extracted from JWT claims stored in the request context by
// auth.JWTMiddleware.
func AuditMiddleware(logger *AuditLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only audit state-changing methods.
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			default:
				next.ServeHTTP(w, r)
				return
			}

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			// Extract values from request before spawning goroutine
			// (request context may be cancelled after handler returns).
			var userIDStr string
			if claims, ok := auth.GetUserFromContext(r.Context()); ok {
				userIDStr = claims.UserID
			}
			method := r.Method
			path := r.URL.Path
			query := r.URL.RawQuery
			ipAddress := r.RemoteAddr
			userAgent := r.UserAgent()
			statusCode := sw.status

			// Fire-and-forget audit logging with independent context.
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				details := map[string]any{
					"method":      method,
					"status_code": statusCode,
				}
				if query != "" {
					details["query"] = query
				}

				action := fmt.Sprintf("%s %s", method, path)

				// Use a zero UUID when no authenticated user is present.
				uid := uuid.Nil
				if userIDStr != "" {
					parsed, err := uuid.Parse(userIDStr)
					if err != nil {
						return
					}
					uid = parsed
				}

				_ = logger.Log(ctx, uid, action, path, details, ipAddress, userAgent)
			}()
		})
	}
}
