package core

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	outpost "github.com/romashqua/outpost"
	"github.com/romashqua/outpost/internal/analytics"
	"github.com/romashqua/outpost/internal/auth"
	"github.com/romashqua/outpost/internal/auth/mfa"
	"github.com/romashqua/outpost/internal/auth/oidc"
	"github.com/romashqua/outpost/internal/auth/saml"
	"github.com/romashqua/outpost/internal/auth/scim"
	"github.com/romashqua/outpost/internal/compliance"
	"github.com/romashqua/outpost/internal/config"
	"github.com/romashqua/outpost/internal/core/handler"
	"github.com/romashqua/outpost/internal/mail"
	"github.com/romashqua/outpost/internal/observability"
	"github.com/romashqua/outpost/internal/session"
	"github.com/romashqua/outpost/internal/webhook"
)

type Server struct {
	cfg        *config.Config
	pool       *pgxpool.Pool
	mailer     *mail.Mailer
	httpServer *http.Server
	grpcServer *grpc.Server
	logger     *slog.Logger
}

func NewServer(cfg *config.Config, pool *pgxpool.Pool, logger *slog.Logger) *Server {
	var mailer *mail.Mailer
	if cfg.SMTP.Host != "" {
		mailer = mail.NewMailer(mail.Config{
			SMTPHost:    cfg.SMTP.Host,
			SMTPPort:    cfg.SMTP.Port,
			FromAddress: cfg.SMTP.From,
			FromName:    cfg.SMTP.FromName,
			Username:    cfg.SMTP.Username,
			Password:    cfg.SMTP.Password,
			TLS:         cfg.SMTP.TLS,
		}, logger)
	}

	return &Server{
		cfg:    cfg,
		pool:   pool,
		mailer: mailer,
		logger: logger,
	}
}

func (s *Server) Start(ctx context.Context) error {
	router := s.setupHTTPRouter()
	s.httpServer = &http.Server{
		Addr:              s.cfg.Server.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	s.grpcServer = s.setupGRPCServer()

	errCh := make(chan error, 2)

	// Start HTTP server.
	go func() {
		s.logger.Info("starting HTTP server", "addr", s.cfg.Server.HTTPAddr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	// Start gRPC server.
	go func() {
		lis, err := net.Listen("tcp", s.cfg.Server.GRPCAddr)
		if err != nil {
			errCh <- fmt.Errorf("grpc listen: %w", err)
			return
		}
		s.logger.Info("starting gRPC server", "addr", s.cfg.Server.GRPCAddr)
		if err := s.grpcServer.Serve(lis); err != nil {
			errCh <- fmt.Errorf("grpc server: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.Shutdown()
	}
}

func (s *Server) Shutdown() error {
	s.logger.Info("shutting down servers")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	s.grpcServer.GracefulStop()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}

	return nil
}

func (s *Server) setupHTTPRouter() chi.Router {
	r := chi.NewRouter()

	// Global middleware.
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	// Audit logging middleware (logs POST/PUT/PATCH/DELETE).
	auditLogger := observability.NewAuditLogger(s.pool)
	r.Use(observability.AuditMiddleware(auditLogger))

	// Metrics endpoint (no auth).
	r.Handle("/metrics", promhttp.Handler())

	// Health endpoints (no auth).
	health := handler.NewHealthHandler(s.pool)
	r.Mount("/", health.Routes())

	// Serve OpenAPI spec (no auth).
	r.Get("/api/docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(outpost.OpenAPISpec)
	})

	// OIDC provider (no auth — public endpoints for relying parties).
	oidcProvider := oidc.NewProvider(s.pool, s.cfg.OIDC.Issuer, nil)
	r.Mount("/oidc", oidcProvider.Routes())

	// SAML 2.0 SP endpoints (no auth — IDP-initiated callbacks).
	if s.cfg.SAML.Enabled {
		samlSP := saml.NewServiceProvider(saml.Config{
			EntityID:       s.cfg.SAML.EntityID,
			ACSURL:         s.cfg.SAML.ACSURL,
			IDPMetadataURL: s.cfg.SAML.IDPMetadataURL,
			CertFile:       s.cfg.SAML.CertFile,
			KeyFile:        s.cfg.SAML.KeyFile,
		}, s.pool, s.logger)
		r.Mount("/saml", samlSP.Routes())
	}

	// API routes.
	r.Route("/api/v1", func(r chi.Router) {
		// Auth endpoints (no JWT required).
		authHandler := handler.NewAuthHandler(s.pool, s.cfg.Auth.JWTSecret)
		r.Mount("/auth", authHandler.Routes())

		// Dashboard stats (no JWT for now — protected by API prefix).
		r.Mount("/dashboard", handler.NewDashboardHandler(s.pool).Routes())

		// SCIM 2.0 provisioning (bearer token auth handled internally).
		r.Mount("/scim/v2", scim.NewHandler(s.pool, s.logger).Routes())

		// Protected routes.
		r.Group(func(r chi.Router) {
			r.Use(auth.JWTMiddleware(s.cfg.Auth.JWTSecret))

			userHandlerOpts := []handler.Mailer{}
			if s.mailer != nil {
				userHandlerOpts = append(userHandlerOpts, s.mailer)
			}
			r.Mount("/users", handler.NewUserHandler(s.pool, s.logger, userHandlerOpts...).Routes())
			r.Mount("/networks", handler.NewNetworkHandler(s.pool, s.logger).Routes())
			r.Mount("/devices", handler.NewDeviceHandler(s.pool, s.logger).Routes())
			r.Mount("/gateways", handler.NewGatewayHandler(s.pool, s.logger).Routes())

			// MFA management.
			mfaMgr := mfa.NewManager(s.pool)
			mfaWebauthn := mfa.NewWebAuthnStore(s.pool)
			r.Mount("/mfa", mfa.NewHandler(mfaMgr, mfaWebauthn).Routes())

			// Session management.
			sessionStore := session.NewMemoryStore()
			sessionMgr := session.NewManager(sessionStore, s.pool, s.cfg.Auth.SessionTTL, s.logger)
			r.Mount("/sessions", sessionMgr.Routes())

			// Audit log viewer.
			r.Mount("/audit", observability.NewAuditHandler(s.pool).Routes())

			// Webhooks.
			r.Mount("/webhooks", webhook.NewDispatcher(s.pool, s.logger).Routes())

			// S2S tunnel management.
			r.Mount("/s2s-tunnels", handler.NewS2SHandler(s.pool, s.logger).Routes())

			// Settings management.
			r.Mount("/settings", handler.NewSettingsHandler(s.pool, s.mailer).Routes())

			// Mail test endpoint.
			r.Mount("/mail", handler.NewMailHandler(s.mailer).Routes())

			// Smart routing (selective proxy bypass).
			r.Mount("/smart-routes", handler.NewSmartRouteHandler(s.pool).Routes())

			// Killer feature routes.
			r.Mount("/analytics", analytics.NewHandler(s.pool).Routes())
			r.Mount("/compliance", compliance.NewHandler(s.pool).Routes())
		})
	})

	// Serve embedded frontend (SPA with fallback to index.html).
	frontendFS, err := fs.Sub(outpost.WebUI, "web-ui/dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(frontendFS))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			// Try to serve the exact file first.
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}
			if f, err := frontendFS.Open(path); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
			// Fallback to index.html for SPA client-side routing.
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	}

	return r
}

// TestableRouter returns the HTTP router for use in tests (e.g. httptest).
func (s *Server) TestableRouter() http.Handler {
	return s.setupHTTPRouter()
}

func (s *Server) setupGRPCServer() *grpc.Server {
	srv := grpc.NewServer()
	registerGatewayService(srv, s.pool, s.logger)
	return srv
}
