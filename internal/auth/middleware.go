package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey int

const claimsKey contextKey = iota

// GetUserFromContext extracts TokenClaims stored by JWTMiddleware.
func GetUserFromContext(ctx context.Context) (*TokenClaims, bool) {
	claims, ok := ctx.Value(claimsKey).(*TokenClaims)
	return claims, ok
}

// JWTMiddleware returns middleware that validates a Bearer token in the
// Authorization header and stores the parsed claims in the request context.
// Responds with 401 Unauthorized on any failure.
func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				respondAuthError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			tokenStr, ok := strings.CutPrefix(header, "Bearer ")
			if !ok {
				respondAuthError(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			claims, err := ValidateToken(secret, tokenStr)
			if err != nil {
				respondAuthError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin is middleware that checks whether the authenticated user is an
// admin. It must be placed after JWTMiddleware in the chain. Returns 403 if
// the user is not an admin, or 401 if no user is found in the context.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetUserFromContext(r.Context())
		if !ok {
			respondAuthError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		if !claims.IsAdmin {
			respondAuthError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// respondAuthError writes a JSON error response matching the API error contract.
func respondAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message, "message": message})
}
