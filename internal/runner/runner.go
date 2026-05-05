package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Result contains the outcome of a command run.
type Result struct {
	ExitCode      int
	Stdout        string
	Stderr        string
	ExecutionTime string
	Error         error
}

// Run executes the specified command with a timeout.
func Run(ctx context.Context, command string, args []string, timeout int) *Result {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	executionTime := time.Since(startTime).String()

	result := &Result{
		Stdout:        strings.TrimSpace(stdout.String()),
		Stderr:        strings.TrimSpace(stderr.String()),
		ExecutionTime: executionTime,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Errorf("command execution timed out")
			result.ExitCode = -1
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = fmt.Errorf("command execution failed: %w", err)
			result.ExitCode = -2
		}
	} else {
		result.ExitCode = 0
	}

	return result
}
