package main

import (
	"os"
	"os/exec"
	"testing"
)

// focusHelperEnv guards the re-exec'd test binary so it runs handleFocus and
// exits — the standard Go idiom for testing os.Exit-calling code without
// killing the parent test process.
const focusHelperEnv = "AGENTDECK_FOCUS_HELPER"

// TestFocusHelperProcess is the subprocess entrypoint. When the guard env is
// set it dispatches to handleFocus (which os.Exit()s with the CLI's chosen
// code) using the argv after the conventional "--" separator. A no-op for
// normal test runs.
//
// The child re-runs the package TestMain, so it gets its OWN isolated temp HOME
// (distinct from the parent's). That is fine here: these tests assert the
// CLI's EXIT CODE — the file round-trip itself is covered exhaustively by
// internal/session/focusrequest_test.go.
func TestFocusHelperProcess(t *testing.T) {
	if os.Getenv(focusHelperEnv) != "1" {
		return
	}
	args := []string{}
	seen := false
	for _, a := range os.Args {
		if seen {
			args = append(args, a)
		}
		if a == "--" {
			seen = true
		}
	}
	handleFocus("", args)
	os.Exit(0) // unreachable: handleFocus always exits.
}

// runFocusSubprocess re-execs the test binary into TestFocusHelperProcess with
// the supplied focus args, returning the child's exit code.
func runFocusSubprocess(t *testing.T, focusArgs ...string) int {
	t.Helper()
	cmdArgs := append([]string{"-test.run=TestFocusHelperProcess", "--"}, focusArgs...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), focusHelperEnv+"=1")

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		t.Fatalf("subprocess failed to run: %v", err)
	}
	return 0
}

// TestHandleFocus_ValidIDExitsZero asserts the happy path: a valid sessionID
// resolves the dir, writes the request, and exits 0.
func TestHandleFocus_ValidIDExitsZero(t *testing.T) {
	if code := runFocusSubprocess(t, "sess-focus-cmd-1"); code != 0 {
		t.Fatalf("focus with valid id must exit 0, got %d", code)
	}
}

// TestHandleFocus_MissingArgExitsNonZero asserts a missing sessionID exits
// non-zero (usage error), matching the CLI's arg-required convention.
func TestHandleFocus_MissingArgExitsNonZero(t *testing.T) {
	if code := runFocusSubprocess(t /* no args */); code == 0 {
		t.Fatal("focus with no session id must exit non-zero")
	}
}
