package executor

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/zinrai/savalet/internal/config"
)

// Executor hosts the command-running process behind a Unix domain socket.
type Executor struct {
	config     *config.ExecutorConfig
	httpServer *http.Server
	listener   net.Listener
}

// New creates a new Executor instance.
func New(config *config.ExecutorConfig) *Executor {
	return &Executor{config: config}
}

// Start binds the Unix domain socket and serves HTTP requests until a signal is received.
func (e *Executor) Start() error {
	if err := os.Remove(e.config.SocketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	listener, err := net.Listen("unix", e.config.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	e.listener = listener

	if e.config.SocketPermissions != "" {
		mode, err := strconv.ParseUint(e.config.SocketPermissions, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid socket permissions: %w", err)
		}
		if err := os.Chmod(e.config.SocketPath, os.FileMode(mode)); err != nil {
			return fmt.Errorf("failed to set socket permissions: %w", err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", e.healthHandler)
	mux.HandleFunc("/execute", e.executeHandler)

	e.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      time.Duration(e.config.MaxExecutionTime+10) * time.Second,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		log.Printf("Executor listening on %s", e.config.SocketPath)
		if err := e.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case sig := <-sigChan:
		log.Printf("Received signal: %v", sig)
		return e.Shutdown()
	case err := <-errChan:
		return fmt.Errorf("HTTP server error: %w", err)
	}
}

// Shutdown gracefully stops the Executor.
func (e *Executor) Shutdown() error {
	log.Println("Shutting down executor...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if e.httpServer != nil {
		if err := e.httpServer.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down HTTP server: %v", err)
		}
	}

	if err := os.Remove(e.config.SocketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("Error removing socket file: %v", err)
	}

	log.Println("Executor shutdown complete")
	return nil
}
