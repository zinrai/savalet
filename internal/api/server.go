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
	grpcclient "github.com/zinrai/savalet/internal/grpc"
	"github.com/zinrai/savalet/internal/models"
)

// Server represents the HTTP API server
type Server struct {
	config     *config.APIConfig
	httpServer *http.Server
	grpcClient *grpcclient.Client
}

// New creates a new API server instance
func New(config *config.APIConfig) *Server {
	return &Server{
		config: config,
	}
}

// Start starts the API server
func (s *Server) Start() error {
	// Connect to daemon via Unix domain socket
	grpcClient, err := grpcclient.NewClient(s.config.SocketPath)
	if err != nil {
		log.Printf("WARNING: Cannot connect to daemon at %s: %v", s.config.SocketPath, err)
		log.Printf("API server will start, but requests will fail until daemon is available")
		// Don't fail here, allow API to start
	} else {
		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := grpcClient.TestConnection(ctx)
		cancel()
		if err != nil {
			log.Printf("WARNING: Daemon connection test failed: %v", err)
		} else {
			log.Printf("Successfully connected to daemon at %s", s.config.SocketPath)
		}
		s.grpcClient = grpcClient
	}

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/ready", s.readyHandler)
	mux.HandleFunc("/execute", s.executeHandler)

	// Wrap with logging middleware
	handler := s.loggingMiddleware(mux)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:           s.config.ListenAddress,
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   time.Duration(s.config.RequestTimeout+10) * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		log.Printf("API server listening on %s", s.config.ListenAddress)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for signal or error
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

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down HTTP server: %v", err)
	}

	// Close gRPC client
	if s.grpcClient != nil {
		if err := s.grpcClient.Close(); err != nil {
			log.Printf("Error closing gRPC client: %v", err)
		}
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

	// Check if we have a gRPC client
	if s.grpcClient == nil {
		// Try to reconnect
		grpcClient, err := grpcclient.NewClient(s.config.SocketPath)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Daemon connection failed"))
			return
		}
		s.grpcClient = grpcClient
	}

	// Test daemon connection
	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()

	err := s.grpcClient.TestConnection(ctx)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Daemon not ready"))
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

	// Check body size
	r.Body = http.MaxBytesReader(w, r.Body, int64(s.config.MaxBodySize))

	// Parse request
	request, err := models.NewExecuteRequestFromJSON(r.Body)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Basic validation
	if err := request.Validate(); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Ensure we have a gRPC connection
	if s.grpcClient == nil {
		grpcClient, err := grpcclient.NewClient(s.config.SocketPath)
		if err != nil {
			s.respondWithError(w, http.StatusServiceUnavailable, "Daemon connection failed")
			return
		}
		s.grpcClient = grpcClient
	}

	// Forward to daemon
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(request.Timeout)*time.Second)
	defer cancel()

	resp, err := s.grpcClient.Execute(ctx, request.Command, request.Args, request.Timeout)
	if err != nil {
		s.respondWithError(w, http.StatusServiceUnavailable, "Failed to execute command")
		return
	}

	// Build HTTP response
	httpResp := models.HTTPResponse{
		Success:       resp.Success,
		ExitCode:      int(resp.ExitCode),
		Stdout:        resp.Stdout,
		Stderr:        resp.Stderr,
		ExecutionTime: resp.ExecutionTime,
	}

	if !resp.Success {
		// Simplify error message for security
		if resp.ErrorMessage == "command not allowed" || resp.ErrorMessage == "argument not allowed" {
			httpResp.Error = resp.ErrorMessage
		} else {
			httpResp.Error = "Command execution failed"
		}
	}

	// Send response
	s.respondWithJSON(w, http.StatusOK, httpResp)
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

		// Wrap ResponseWriter to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Process request
		next.ServeHTTP(wrapped, r)

		// Log request
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

	// Check log level
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
