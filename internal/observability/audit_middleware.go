package observability

import (
	"fmt"
	"net/http"

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

			// Fire-and-forget audit logging after the request completes.
			go func() {
				ctx := r.Context()

				var userIDStr string
				claims, ok := auth.GetUserFromContext(ctx)
				if ok {
					userIDStr = claims.UserID
				}

				details := map[string]any{
					"method":      r.Method,
					"status_code": sw.status,
				}
				if r.URL.RawQuery != "" {
					details["query"] = r.URL.RawQuery
				}

				action := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
				resource := r.URL.Path
				ipAddress := r.RemoteAddr
				userAgent := r.UserAgent()

				// Use a zero UUID when no authenticated user is present.
				uid := uuid.Nil
				if userIDStr != "" {
					parsed, err := uuid.Parse(userIDStr)
					if err != nil {
						return
					}
					uid = parsed
				}

				_ = logger.Log(ctx, uid, action, resource, details, ipAddress, userAgent)
			}()
		})
	}
}
