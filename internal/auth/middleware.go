package auth

import (
	"context"
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
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			tokenStr, ok := strings.CutPrefix(header, "Bearer ")
			if !ok {
				http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
				return
			}

			claims, err := ValidateToken(secret, tokenStr)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
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
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		if !claims.IsAdmin {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
