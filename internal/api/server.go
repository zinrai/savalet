package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zinrai/savalet/internal/config"
	"github.com/zinrai/savalet/internal/models"
)

// Server represents the HTTP API server
type Server struct {
	config         *config.APIConfig
	httpServer     *http.Server
	executorClient *ExecutorClient
}

// New creates a new API server instance
func New(config *config.APIConfig) *Server {
	return &Server{
		config:         config,
		executorClient: NewExecutorClient(config.SocketPath),
	}
}

// Start starts the API server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/ready", s.readyHandler)
	mux.HandleFunc("/execute", s.executeHandler)

	handler := s.loggingMiddleware(mux)

	s.httpServer = &http.Server{
		Addr:           s.config.ListenAddress,
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   time.Duration(s.config.RequestTimeout+10) * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		log.Printf("API server listening on %s", s.config.ListenAddress)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case sig := <-sigChan:
		log.Printf("Received signal: %v", sig)
		return s.Shutdown()
	case err := <-errChan:
		return fmt.Errorf("HTTP server error: %w", err)
	}
}

// Shutdown gracefully shuts down the API server
func (s *Server) Shutdown() error {
	log.Println("Shutting down API server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down HTTP server: %v", err)
	}

	if s.executorClient != nil {
		s.executorClient.Close()
	}

	log.Println("API server shutdown complete")
	return nil
}

// healthHandler handles /health endpoint
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// readyHandler handles /ready endpoint
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.executorClient.Health(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Executor unreachable"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

// executeHandler handles /execute endpoint
func (s *Server) executeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(s.config.MaxBodySize))

	request, err := models.NewExecuteRequestFromJSON(r.Body)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if err := request.Validate(); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(request.Timeout+5)*time.Second)
	defer cancel()

	resp, status, err := s.executorClient.Execute(ctx, request)
	if err != nil {
		s.respondWithError(w, http.StatusServiceUnavailable, "Failed to reach executor")
		return
	}

	s.respondWithJSON(w, status, resp)
}

// respondWithJSON sends a JSON response
func (s *Server) respondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(response)
}

// respondWithError sends an error response
func (s *Server) respondWithError(w http.ResponseWriter, status int, message string) {
	resp := models.HTTPResponse{
		Success: false,
		Error:   message,
	}
	s.respondWithJSON(w, status, resp)
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)

		latency := time.Since(start)
		logEntry := models.LogEntry{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Level:      "info",
			Mode:       "api",
			Event:      "http_request",
			Method:     r.Method,
			Path:       r.URL.Path,
			RemoteAddr: r.RemoteAddr,
			Status:     wrapped.statusCode,
			Latency:    latency.String(),
		}

		s.logJSON(logEntry)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// logJSON logs an entry in JSON format
func (s *Server) logJSON(entry models.LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Failed to marshal log entry: %v", err)
		return
	}

	switch entry.Level {
	case "debug":
		if s.config.LogLevel != "debug" {
			return
		}
	case "info":
		if s.config.LogLevel == "warn" || s.config.LogLevel == "error" {
			return
		}
	case "warn":
		if s.config.LogLevel == "error" {
			return
		}
	}

	log.Println(string(data))
}
