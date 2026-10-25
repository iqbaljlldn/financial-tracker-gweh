package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type contextKey string

const RequestIDKey contextKey = "platform.requestID"

// Config contains settings for the HTTP server.
type Config struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// Server wraps the standard http.Server with graceful lifecycle controls.
type Server struct {
	httpServer *http.Server
	router     *chi.Mux
	logger     *slog.Logger
}

// NewServer initializes a new Server with the standard middleware chain.
func NewServer(cfg Config, logger *slog.Logger) *Server {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 15 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 15 * time.Second
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 60 * time.Second
	}

	r := chi.NewRouter()

	// 1. Request ID Middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			reqID := req.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = "req_" + uuid.New().String()[:12]
			}
			w.Header().Set("X-Request-ID", reqID)
			ctx := context.WithValue(req.Context(), RequestIDKey, reqID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	// 2. Structured Logging Middleware with slog
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)

			defer func() {
				reqID, _ := req.Context().Value(RequestIDKey).(string)
				logger.Info("http request",
					slog.String("method", req.Method),
					slog.String("path", req.URL.Path),
					slog.Int("status", ww.Status()),
					slog.Duration("latency", time.Since(start)),
					slog.String("requestId", reqID),
					slog.String("remoteAddr", req.RemoteAddr),
				)
			}()

			next.ServeHTTP(ww, req)
		})
	})

	// 3. Panic Recovery Middleware (09_api_specification.md §7.2 error envelope)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					reqID, _ := req.Context().Value(RequestIDKey).(string)
					logger.Error("panic recovered in HTTP handler",
						slog.Any("error", rec),
						slog.String("stack", string(debug.Stack())),
						slog.String("requestId", reqID),
					)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(errorResponse{
						Error: errorDetail{
							Code:      "INTERNAL_ERROR",
							Message:   "An unexpected server error occurred",
							RequestID: reqID,
						},
					})
				}
			}()
			next.ServeHTTP(w, req)
		})
	})

	// 4. CORS Middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")

			if req.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	// 5. Health checks
	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &Server{
		httpServer: srv,
		router:     r,
		logger:     logger,
	}
}

// Router returns the chi router for mounting module endpoints.
func (s *Server) Router() *chi.Mux {
	return s.router
}

// Start begins listening on the configured port.
func (s *Server) Start() error {
	s.logger.Info("http server listening", slog.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down http server gracefully")
	return s.httpServer.Shutdown(ctx)
}

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}
