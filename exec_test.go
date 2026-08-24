package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func runExec(t *testing.T, timeout, maxOutput int, argv ...string) execResult {
	t.Helper()
	a := &Action{Timeout: timeout}
	return executeAction(context.Background(), a, argv, maxOutput)
}

func TestExecReportsWhatTheCommandDid(t *testing.T) {
	res := runExec(t, 5, 1024, "/bin/sh", "-c", "echo out; echo err >&2; exit 3")
	if res.Status != statusCompleted {
		t.Errorf("Status = %q, want %q", res.Status, statusCompleted)
	}
	if res.ExitCode == nil || *res.ExitCode != 3 {
		t.Errorf("ExitCode = %v, want 3", res.ExitCode)
	}
	if res.Stdout != "out\n" || res.Stderr != "err\n" {
		t.Errorf("Stdout, Stderr = %q, %q, want %q, %q", res.Stdout, res.Stderr, "out\n", "err\n")
	}
}

func TestExecTruncatesOutput(t *testing.T) {
	res := runExec(t, 5, 100, "/bin/sh", "-c", "head -c 10000 /dev/zero")
	if len(res.Stdout) != 100 {
		t.Errorf("len(Stdout) = %d, want 100", len(res.Stdout))
	}
	if !res.StdoutTruncated {
		t.Error("StdoutTruncated = false, want true")
	}
	if res.Status != statusCompleted {
		t.Errorf("Status = %q, want %q (writer must not error on overflow)", res.Status, statusCompleted)
	}
}

func TestExecTimeoutTermsProcessGroup(t *testing.T) {
	// The background sleep is a grandchild holding the stdout pipe. If
	// SIGTERM reached only the direct child, the run would block for the
	// full WaitDelay after the timeout. Group delivery ends it promptly.
	started := time.Now()
	res := runExec(t, 1, 1024, "/bin/sh", "-c", "sleep 30 & wait")
	elapsed := time.Since(started)

	if res.Status != statusTimeout {
		t.Errorf("Status = %q, want %q", res.Status, statusTimeout)
	}
	if elapsed >= 1*time.Second+termGracePeriod {
		t.Errorf("run took %v, group SIGTERM should end it before WaitDelay expires", elapsed)
	}
}

func TestExecTimeoutTermIsCatchable(t *testing.T) {
	res := runExec(t, 1, 1024, "/bin/sh", "-c", `trap 'exit 0' TERM; sleep 30 & wait`)
	if res.Status != statusTimeout {
		t.Errorf("Status = %q, want %q", res.Status, statusTimeout)
	}
	if res.ExitCode == nil || *res.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0 (process should exit via its TERM trap)", res.ExitCode)
	}
}

// A command that never ran must not be reported as having succeeded.
func TestExecSpawnFailed(t *testing.T) {
	res := runExec(t, 5, 1024, "/nonexistent/savalet-test-binary")
	if res.Status != statusSpawnFailed {
		t.Errorf("Status = %q, want %q", res.Status, statusSpawnFailed)
	}
	if res.ExitCode != nil {
		t.Errorf("ExitCode = %v, want nil", *res.ExitCode)
	}
	if res.SpawnErr == nil || !strings.Contains(res.SpawnErr.Error(), "no such file") {
		t.Errorf("SpawnErr = %v, want exec error", res.SpawnErr)
	}
}
