package daemon

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/zinrai/savalet/internal/config"
	grpcsrv "github.com/zinrai/savalet/internal/grpc"
	"github.com/zinrai/savalet/pb"
	"google.golang.org/grpc"
)

// Daemon represents the mediator daemon
type Daemon struct {
	config     *config.DaemonConfig
	grpcServer *grpc.Server
	listener   net.Listener
}

// New creates a new daemon instance
func New(config *config.DaemonConfig) *Daemon {
	return &Daemon{
		config: config,
	}
}

// Start starts the daemon and listens for requests
func (d *Daemon) Start() error {
	// Remove existing socket file if it exists
	if err := os.Remove(d.config.SocketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", d.config.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	d.listener = listener

	// Set socket permissions
	if d.config.SocketPermissions != "" {
		mode, err := strconv.ParseUint(d.config.SocketPermissions, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid socket permissions: %w", err)
		}
		if err := os.Chmod(d.config.SocketPath, os.FileMode(mode)); err != nil {
			return fmt.Errorf("failed to set socket permissions: %w", err)
		}
	}

	// Create gRPC server
	d.grpcServer = grpc.NewServer(
		grpc.MaxRecvMsgSize(1024*1024),    // 1MB
		grpc.MaxSendMsgSize(10*1024*1024), // 10MB for command output
	)

	// Register gRPC service
	grpcService := grpcsrv.NewServer(d.config)
	pb.RegisterCommandExecutorServer(d.grpcServer, grpcService)

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start serving in goroutine
	errChan := make(chan error, 1)
	go func() {
		log.Printf("Daemon listening on %s", d.config.SocketPath)
		if err := d.grpcServer.Serve(listener); err != nil {
			errChan <- err
		}
	}()

	// Wait for signal or error
	select {
	case sig := <-sigChan:
		log.Printf("Received signal: %v", sig)
		return d.Shutdown()
	case err := <-errChan:
		return fmt.Errorf("gRPC server error: %w", err)
	}
}

// Shutdown gracefully shuts down the daemon
func (d *Daemon) Shutdown() error {
	log.Println("Shutting down daemon...")

	// Stop accepting new connections
	if d.grpcServer != nil {
		d.grpcServer.GracefulStop()
	}

	// Close listener
	if d.listener != nil {
		if err := d.listener.Close(); err != nil {
			log.Printf("Error closing listener: %v", err)
		}
	}

	// Remove socket file
	if err := os.Remove(d.config.SocketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("Error removing socket file: %v", err)
	}

	log.Println("Daemon shutdown complete")
	return nil
}
