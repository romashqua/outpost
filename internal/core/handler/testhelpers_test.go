package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/romashqua/outpost/internal/auth"
)

// adminCtx injects admin JWT claims into the request context,
// bypassing JWTMiddleware and RequireAdmin for handler unit tests.
func adminCtx(r *http.Request) *http.Request {
	return reqWithAdminClaims(r)
}

// reqWithAdminClaims injects admin JWT claims into the request context,
// bypassing JWTMiddleware and RequireAdmin for handler unit tests.
func reqWithAdminClaims(r *http.Request) *http.Request {
	claims := &auth.TokenClaims{
		UserID:   uuid.New().String(),
		Username: "admin",
		Email:    "admin@test.local",
		IsAdmin:  true,
		Roles:    []string{"admin"},
	}
	ctx := auth.ContextWithClaims(r.Context(), claims)
	return r.WithContext(ctx)
}

// reqWithUserClaims injects non-admin JWT claims into the request context.
func reqWithUserClaims(r *http.Request, userID string) *http.Request {
	claims := &auth.TokenClaims{
		UserID:   userID,
		Username: "testuser",
		Email:    "user@test.local",
		IsAdmin:  false,
		Roles:    []string{"user"},
	}
	ctx := auth.ContextWithClaims(r.Context(), claims)
	return r.WithContext(ctx)
}
