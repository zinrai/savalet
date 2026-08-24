package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const termGracePeriod = 5 * time.Second

const (
	statusCompleted   = "completed"
	statusTimeout     = "timeout"
	statusSignaled    = "signaled"
	statusSpawnFailed = "spawn_failed"
)

// The child environment is fixed rather than inherited: savalet's own
// environment is an artifact of how it was started, and LANG=C keeps
// command output locale independent.
var childEnv = []string{
	"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	"LANG=C",
}

type execResult struct {
	Status          string
	ExitCode        *int
	SpawnErr        error
	Stdout          string
	Stderr          string
	StdoutTruncated bool
	StderrTruncated bool
	StartedAt       time.Time
	Duration        time.Duration
}

// ctx must be the process root context, not the HTTP request context: a
// client disconnect must not interrupt a command that has side effects.
func executeAction(ctx context.Context, a *Action, argv []string, maxOutputBytes int) execResult {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(a.Timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = childEnv
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Not SIGTERM to the direct child alone: a child that spawns its own
	// children would leave them running past the timeout. The SIGKILL
	// after WaitDelay reaches only the direct child, so a grandchild that
	// ignores SIGTERM can survive, but WaitDelay closes the output pipes
	// so it cannot hold up the response.
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = termGracePeriod

	stdout := &limitedBuffer{limit: maxOutputBytes}
	stderr := &limitedBuffer{limit: maxOutputBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err := cmd.Run()

	res := execResult{
		Stdout:          stdout.buf.String(),
		Stderr:          stderr.buf.String(),
		StdoutTruncated: stdout.truncated,
		StderrTruncated: stderr.truncated,
		StartedAt:       started,
		Duration:        time.Since(started),
	}

	if cmd.ProcessState == nil {
		res.Status = statusSpawnFailed
		res.SpawnErr = err
		return res
	}
	if code := cmd.ProcessState.ExitCode(); code >= 0 {
		res.ExitCode = &code
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Status = statusTimeout
	case errors.Is(ctx.Err(), context.Canceled):
		res.Status = statusSignaled
	case res.ExitCode == nil:
		res.Status = statusSignaled
	default:
		res.Status = statusCompleted
	}
	return res
}

// Returning an error on overflow would kill the writing command with
// EPIPE, so the excess is reported as written and discarded.
type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remain := b.limit - b.buf.Len()
	if remain <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remain {
		b.buf.Write(p[:remain])
		b.truncated = true
		return len(p), nil
	}
	return b.buf.Write(p)
}
