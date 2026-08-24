package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Request bodies carry only enum values, so they never need to be large.
const maxRequestBodyBytes = 64 << 10

type server struct {
	cfg    *Config
	logger *slog.Logger
	// Not the request context: see executeAction.
	rootCtx context.Context
}

type actionResponse struct {
	RequestID       string            `json:"request_id"`
	Action          string            `json:"action"`
	Params          map[string]string `json:"params"`
	Status          string            `json:"status"`
	ExitCode        *int              `json:"exit_code"`
	Stdout          string            `json:"stdout"`
	Stderr          string            `json:"stderr"`
	StdoutTruncated bool              `json:"stdout_truncated"`
	StderrTruncated bool              `json:"stderr_truncated"`
	StartedAt       string            `json:"started_at"`
	DurationMS      int64             `json:"duration_ms"`
}

type errorResponse struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error"`
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /actions/{name}", s.handleAction)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}

func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

func (s *server) handleAction(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r)
	name := r.PathValue("name")

	action, ok := s.cfg.byName[name]
	if !ok {
		s.reject(w, r, requestID, name, nil, http.StatusNotFound,
			fmt.Sprintf("unknown action: %s", capString(name)))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	values, err := decodeParams(r.Body)
	if err != nil {
		status := http.StatusBadRequest
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			status = http.StatusRequestEntityTooLarge
		}
		s.reject(w, r, requestID, name, nil, status, err.Error())
		return
	}

	if err := action.ValidateParams(values); err != nil {
		s.reject(w, r, requestID, name, values, http.StatusBadRequest, err.Error())
		return
	}

	argv := action.BuildArgv(values)
	res := executeAction(s.rootCtx, action, argv, s.cfg.MaxOutputBytes)

	attrs := []any{
		"request_id", requestID,
		"action", name,
		"params", values,
		"argv", argv,
		"status", res.Status,
		"exit_code", res.ExitCode,
		"duration_ms", res.Duration.Milliseconds(),
	}
	if res.SpawnErr != nil {
		attrs = append(attrs, "error", res.SpawnErr.Error())
	}
	attrs = s.appendIdentity(attrs, r)
	s.logger.Info("action_executed", attrs...)

	httpStatus := http.StatusOK
	if res.Status == statusSpawnFailed {
		httpStatus = http.StatusInternalServerError
	}
	writeJSON(w, httpStatus, actionResponse{
		RequestID:       requestID,
		Action:          name,
		Params:          values,
		Status:          res.Status,
		ExitCode:        res.ExitCode,
		Stdout:          res.Stdout,
		Stderr:          res.Stderr,
		StdoutTruncated: res.StdoutTruncated,
		StderrTruncated: res.StderrTruncated,
		StartedAt:       res.StartedAt.UTC().Format(time.RFC3339),
		DurationMS:      res.Duration.Milliseconds(),
	})
}

func (s *server) reject(w http.ResponseWriter, r *http.Request, requestID, action string, values map[string]string, status int, msg string) {
	attrs := []any{
		"request_id", requestID,
		"action", capString(action),
		"error", msg,
	}
	if values != nil {
		// Unlike action_executed, these values failed validation and are
		// arbitrary client input, hence the cap.
		attrs = append(attrs, "params", capParams(values))
	}
	attrs = s.appendIdentity(attrs, r)
	s.logger.Info("action_rejected", attrs...)
	writeJSON(w, status, errorResponse{RequestID: requestID, Error: msg})
}

func (s *server) appendIdentity(attrs []any, r *http.Request) []any {
	if s.cfg.IdentityHeader == "" {
		return attrs
	}
	return append(attrs, "identity", capString(r.Header.Get(s.cfg.IdentityHeader)))
}

func decodeParams(body io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return map[string]string{}, nil
	}
	var req struct {
		Params map[string]string `json:"params"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	// An unknown body field is a client bug, and skipping it would hide
	// a misspelled "params" behind a successful execution.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid request body: %v", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("invalid request body: trailing data after JSON object")
	}
	if req.Params == nil {
		return map[string]string{}, nil
	}
	return req.Params, nil
}

func requestIDFrom(r *http.Request) string {
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return capString(id)
	}
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// An encode failure means the client is gone, and there is no one
	// left to tell.
	_ = json.NewEncoder(w).Encode(v)
}

func listen(cfg *Config) (net.Listener, error) {
	if os.Getenv("LISTEN_FDS") == "1" && os.Getenv("LISTEN_PID") == strconv.Itoa(os.Getpid()) {
		f := os.NewFile(3, "savalet.sock")
		return net.FileListener(f)
	}
	if path, ok := strings.CutPrefix(cfg.Listen, "unix:"); ok {
		// Self managed listening exists for development runs only. The
		// unconditional removal can steal the socket from a live process,
		// and the window before chmod is a permission gap. Socket
		// activation has neither problem, which is why it is the
		// deployment path.
		os.Remove(path)
		l, err := net.Listen("unix", path)
		if err != nil {
			return nil, err
		}
		if err := os.Chmod(path, 0o660); err != nil {
			l.Close()
			return nil, err
		}
		return l, nil
	}
	return net.Listen("tcp", cfg.Listen)
}

func (s *server) serve(l net.Listener) error {
	maxTimeout := 0
	for _, a := range s.cfg.Actions {
		if a.Timeout > maxTimeout {
			maxTimeout = a.Timeout
		}
	}
	srv := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		// A handler blocks for at most the longest action timeout, so the
		// write deadline derives from it instead of being configured.
		WriteTimeout: time.Duration(maxTimeout+10) * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(l) }()

	select {
	case err := <-errCh:
		return err
	case <-s.rootCtx.Done():
	}
	// No indefinite wait: in flight commands already received SIGTERM
	// through the context chain and resolve within the grace period, so
	// 3x is margin rather than a cap that can fire.
	sctx, cancel := context.WithTimeout(context.Background(), 3*termGracePeriod)
	defer cancel()
	return srv.Shutdown(sctx)
}
