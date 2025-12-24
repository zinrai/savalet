package grpc

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/zinrai/savalet/internal/config"
	"github.com/zinrai/savalet/internal/executor"
	"github.com/zinrai/savalet/internal/models"
	"github.com/zinrai/savalet/internal/validator"
	"github.com/zinrai/savalet/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the CommandExecutor service
type Server struct {
	pb.UnimplementedCommandExecutorServer
	config *config.DaemonConfig
}

// NewServer creates a new gRPC server instance
func NewServer(config *config.DaemonConfig) pb.CommandExecutorServer {
	return &Server{
		config: config,
	}
}

// Execute handles command execution requests
func (s *Server) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	// Log the request (audit log)
	logEntry := models.LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     "info",
		Mode:      "daemon",
		Event:     "command_request",
		Command:   req.Command,
		Args:      req.Args,
	}

	// Validate command
	err := validator.ValidateCommand(req.Command, req.Args, &s.config.Commands)
	if err != nil {
		logEntry.Event = "command_rejected"
		logEntry.Error = err.Error()
		s.logJSON(logEntry)

		// Return error response (not gRPC error)
		return &pb.ExecuteResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Check timeout limits
	timeout := int(req.Timeout)
	if timeout <= 0 {
		timeout = s.config.DefaultTimeout
	}
	if timeout > s.config.MaxExecutionTime {
		timeout = s.config.MaxExecutionTime
	}

	// Execute command
	result := executor.ExecuteCommand(ctx, req.Command, req.Args, timeout)

	// Log execution result
	logEntry.Event = "command_executed"
	logEntry.ExitCode = result.ExitCode
	logEntry.ExecutionTime = result.ExecutionTime
	if result.Error != nil {
		logEntry.Error = result.Error.Error()
	}
	s.logJSON(logEntry)

	// Build response
	resp := &pb.ExecuteResponse{
		Success:       result.Error == nil,
		ExitCode:      int32(result.ExitCode),
		Stdout:        result.Stdout,
		Stderr:        result.Stderr,
		ExecutionTime: result.ExecutionTime,
	}

	if result.Error != nil {
		resp.ErrorMessage = result.Error.Error()
	}

	return resp, nil
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

// ValidateContext checks if the context is still valid
func ValidateContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return status.Error(codes.DeadlineExceeded, "request timeout")
	default:
		return nil
	}
}
