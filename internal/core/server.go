package core

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	"github.com/romashqua-labs/outpost/internal/auth"
	"github.com/romashqua-labs/outpost/internal/config"
	"github.com/romashqua-labs/outpost/internal/core/handler"
)

type Server struct {
	cfg        *config.Config
	pool       *pgxpool.Pool
	httpServer *http.Server
	grpcServer *grpc.Server
	logger     *slog.Logger
}

func NewServer(cfg *config.Config, pool *pgxpool.Pool, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		pool:   pool,
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

	// Metrics endpoint (no auth).
	r.Handle("/metrics", promhttp.Handler())

	// Health endpoints (no auth).
	health := handler.NewHealthHandler(s.pool)
	r.Mount("/", health.Routes())

	// API routes.
	r.Route("/api/v1", func(r chi.Router) {
		// Auth endpoints (no JWT required).
		authHandler := handler.NewAuthHandler(s.pool, s.cfg.Auth.JWTSecret)
		r.Mount("/auth", authHandler.Routes())

		// Protected routes.
		r.Group(func(r chi.Router) {
			r.Use(auth.JWTMiddleware(s.cfg.Auth.JWTSecret))

			r.Mount("/users", handler.NewUserHandler(s.pool).Routes())
			r.Mount("/networks", handler.NewNetworkHandler(s.pool).Routes())
			r.Mount("/devices", handler.NewDeviceHandler(s.pool).Routes())
			r.Mount("/gateways", handler.NewGatewayHandler(s.pool).Routes())
		})
	})

	return r
}

func (s *Server) setupGRPCServer() *grpc.Server {
	srv := grpc.NewServer()
	// Gateway and proxy services will be registered here.
	return srv
}
