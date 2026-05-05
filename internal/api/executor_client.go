package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/zinrai/savalet/internal/models"
)

// ExecutorClient calls the executor process via HTTP over a Unix domain socket.
type ExecutorClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewExecutorClient builds a client that dials the executor's Unix domain socket.
func NewExecutorClient(socketPath string) *ExecutorClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		IdleConnTimeout: 90 * time.Second,
	}
	return &ExecutorClient{
		httpClient: &http.Client{Transport: transport},
		baseURL:    "http://executor",
	}
}

// Execute forwards the request to the executor and returns the parsed response and HTTP status code.
func (c *ExecutorClient) Execute(ctx context.Context, req *models.ExecuteRequest) (*models.HTTPResponse, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to call executor: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response: %w", err)
	}

	var result models.HTTPResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, resp.StatusCode, nil
}

// Health checks executor reachability via GET /health.
func (c *ExecutorClient) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("executor unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("executor returned status %d", resp.StatusCode)
	}
	return nil
}

// Close releases idle connections held by the underlying transport.
func (c *ExecutorClient) Close() {
	c.httpClient.CloseIdleConnections()
}
