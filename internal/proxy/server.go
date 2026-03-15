package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/romashqua/outpost/internal/config"
)

// Server is the enrollment and auth proxy that sits in the DMZ.
// It forwards unauthenticated enrollment and authentication requests
// to the outpost-core HTTP API.
type Server struct {
	cfg        *config.Config
	logger     *slog.Logger
	http       *http.Server
	coreClient *http.Client
}

// NewServer creates a new proxy server.
func NewServer(cfg *config.Config, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
		coreClient: &http.Client{
			Timeout: 30 * time.Second,
			// Do not follow redirects automatically; let the client handle them.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Run starts the proxy HTTP server and blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(s.auditLogMiddleware)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"outpost-proxy"}`))
	})

	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Check that core is reachable.
		coreHealthURL := s.coreURL("/healthz")
		resp, err := s.coreClient.Get(coreHealthURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"not_ready","reason":"core unreachable"}`))
			return
		}
		resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Enrollment endpoint: accepts enrollment requests from outpost-client
	// and proxies them to core's device enrollment API.
	r.Post("/enroll", s.handleEnroll)

	// Auth proxy routes: forward authentication requests to core.
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", s.proxyToCore("/api/v1/auth/login"))
		r.Post("/mfa/verify", s.proxyToCore("/api/v1/auth/mfa/verify"))
		r.Post("/refresh", s.proxyToCore("/api/v1/auth/refresh"))
		r.Get("/oidc/authorize", s.proxyToCore("/api/v1/auth/oidc/authorize"))
		r.Post("/oidc/callback", s.proxyToCore("/api/v1/auth/oidc/callback"))
	})

	s.http = &http.Server{
		Addr:              s.cfg.Proxy.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("proxy HTTP server listening",
			"addr", s.cfg.Proxy.ListenAddr,
			"core_url", s.cfg.Proxy.CoreURL,
		)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("proxy http: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	}
}

// coreURL builds a full URL to core's HTTP API.
func (s *Server) coreURL(path string) string {
	base := strings.TrimRight(s.cfg.Proxy.CoreURL, "/")
	return base + path
}

// handleEnroll validates the enrollment token from the request body and
// proxies the enrollment to core's device enrollment endpoint.
func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	s.forwardRequest(w, r, s.coreURL("/api/v1/devices/enroll"))
}

// proxyToCore returns an http.HandlerFunc that forwards the incoming request
// to the specified core API path.
func (s *Server) proxyToCore(corePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.forwardRequest(w, r, s.coreURL(corePath))
	}
}

// forwardRequest forwards an HTTP request to the given target URL and writes
// the response back to the client. It copies the request body, relevant
// headers, and the full response from core.
func (s *Server) forwardRequest(w http.ResponseWriter, r *http.Request, targetURL string) {
	// Build the outgoing request to core.
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		s.logger.Error("failed to create proxy request",
			"target", targetURL,
			"error", err,
		)
		http.Error(w, `{"error":"internal proxy error","message":"internal proxy error"}`, http.StatusBadGateway)
		return
	}

	// Copy relevant headers from the original request.
	copyHeaders(r.Header, outReq.Header, []string{
		"Content-Type",
		"Content-Length",
		"Accept",
		"Accept-Language",
		"Authorization",
		"Cookie",
		"User-Agent",
		"X-Forwarded-For",
		"X-Request-Id",
	})

	// Set proxy identification headers.
	outReq.Header.Set("X-Forwarded-By", "outpost-proxy")
	if clientIP := r.RemoteAddr; clientIP != "" {
		existing := outReq.Header.Get("X-Forwarded-For")
		if existing != "" {
			outReq.Header.Set("X-Forwarded-For", existing+", "+clientIP)
		} else {
			outReq.Header.Set("X-Forwarded-For", clientIP)
		}
	}
	if reqID := chimiddleware.GetReqID(r.Context()); reqID != "" {
		outReq.Header.Set("X-Request-Id", reqID)
	}

	// Send request to core.
	resp, err := s.coreClient.Do(outReq)
	if err != nil {
		s.logger.Error("failed to reach core",
			"target", targetURL,
			"error", err,
		)
		http.Error(w, `{"error":"core unreachable","message":"core unreachable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers from core to the client.
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}

	// Write the status code and body.
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		s.logger.Error("failed to write proxy response",
			"target", targetURL,
			"error", err,
		)
	}
}

// copyHeaders copies specific headers from src to dst.
func copyHeaders(src, dst http.Header, keys []string) {
	for _, k := range keys {
		if v := src.Get(k); v != "" {
			dst.Set(k, v)
		}
	}
}

// auditLogMiddleware logs every request for audit purposes.
func (s *Server) auditLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

		s.logger.Info("proxy request started",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"request_id", chimiddleware.GetReqID(r.Context()),
		)

		next.ServeHTTP(ww, r)

		s.logger.Info("proxy request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", chimiddleware.GetReqID(r.Context()),
		)
	})
}
