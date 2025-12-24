package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/zinrai/savalet/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client represents a gRPC client for the daemon
type Client struct {
	conn   *grpc.ClientConn
	client pb.CommandExecutorClient
}

// NewClient creates a new gRPC client
func NewClient(socketPath string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect to Unix domain socket
	target := fmt.Sprintf("unix://%s", socketPath)
	conn, err := grpc.DialContext(ctx, target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewCommandExecutorClient(conn),
	}, nil
}

// Execute sends a command execution request to the daemon
func (c *Client) Execute(ctx context.Context, command string, args []string, timeout int) (*pb.ExecuteResponse, error) {
	req := &pb.ExecuteRequest{
		Command: command,
		Args:    args,
		Timeout: int32(timeout),
	}

	resp, err := c.client.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC call failed: %w", err)
	}

	return resp, nil
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
