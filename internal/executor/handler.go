package executor

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/zinrai/savalet/internal/models"
	"github.com/zinrai/savalet/internal/runner"
	"github.com/zinrai/savalet/internal/validator"
)

func (e *Executor) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (e *Executor) executeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	request, err := models.NewExecuteRequestFromJSON(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if err := request.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	logEntry := models.LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     "info",
		Mode:      "executor",
		Event:     "command_request",
		Command:   request.Command,
		Args:      request.Args,
	}

	if err := validator.ValidateCommand(request.Command, request.Args, &e.config.Commands); err != nil {
		logEntry.Event = "command_rejected"
		logEntry.Error = err.Error()
		e.logJSON(logEntry)
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	timeout := request.Timeout
	if timeout <= 0 {
		timeout = e.config.DefaultTimeout
	}
	if timeout > e.config.MaxExecutionTime {
		timeout = e.config.MaxExecutionTime
	}

	result := runner.Run(r.Context(), request.Command, request.Args, timeout)

	logEntry.Event = "command_executed"
	logEntry.ExitCode = result.ExitCode
	logEntry.ExecutionTime = result.ExecutionTime
	if result.Error != nil {
		logEntry.Error = result.Error.Error()
	}
	e.logJSON(logEntry)

	resp := models.HTTPResponse{
		Success:       result.Error == nil,
		ExitCode:      result.ExitCode,
		Stdout:        result.Stdout,
		Stderr:        result.Stderr,
		ExecutionTime: result.ExecutionTime,
	}

	status := http.StatusOK
	if result.Error != nil {
		if result.ExitCode == -1 {
			resp.Error = "command execution timed out"
		} else {
			resp.Error = "command execution failed"
			status = http.StatusInternalServerError
		}
	}

	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, models.HTTPResponse{
		Success: false,
		Error:   message,
	})
}

func (e *Executor) logJSON(entry models.LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Failed to marshal log entry: %v", err)
		return
	}

	switch entry.Level {
	case "debug":
		if e.config.LogLevel != "debug" {
			return
		}
	case "info":
		if e.config.LogLevel == "warn" || e.config.LogLevel == "error" {
			return
		}
	case "warn":
		if e.config.LogLevel == "error" {
			return
		}
	}

	log.Println(string(data))
}
