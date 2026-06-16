package git

// Tests that remote fetches run non-interactively so a passphrase-protected
// SSH key (not loaded into ssh-agent) can never make the child ssh block on a
// /dev/tty passphrase prompt — which would hang the fetch for the full
// fetchTimeout and corrupt agent-deck's raw-mode TUI. Mirrors the BatchMode=yes
// stance the remote-session layer adopted in #1206.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeSSH writes an executable shell script that records the argv it was
// invoked with to recordPath, then exits 255 (as ssh would when BatchMode
// forbids an interactive prompt). Returns the script path.
func writeFakeSSH(t *testing.T, recordPath string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-ssh.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + recordPath + "'\nexit 255\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	return script
}

// TestFetchRemoteShapedRef_PassesBatchModeToSSH proves the fetch hands ssh
// BatchMode=yes. With it, a passphrase-protected key fails fast instead of
// prompting; without the fix the fetch inherits a bare GIT_SSH_COMMAND and ssh
// is free to block on /dev/tty.
func TestFetchRemoteShapedRef_PassesBatchModeToSSH(t *testing.T) {
	repo := t.TempDir()
	mustGitE1(t, repo, "init", "-b", "main")
	mustGitE1(t, repo, "config", "user.email", "test@test.com")
	mustGitE1(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	mustGitE1(t, repo, "add", ".")
	mustGitE1(t, repo, "commit", "-m", "seed")

	// ssh-shaped (scp-like) URL forces git through GIT_SSH_COMMAND without any
	// network round-trip — the fake ssh is exec'd and records its argv.
	mustGitE1(t, repo, "remote", "add", "origin", "git@fake.invalid:repo.git")

	recordPath := filepath.Join(t.TempDir(), "ssh-argv.txt")
	fakeSSH := writeFakeSSH(t, recordPath)
	t.Setenv("GIT_SSH_COMMAND", fakeSSH)

	warning := fetchRemoteShapedRef(repo, "origin/main")
	if warning == "" {
		t.Fatal("expected fetch to fail (fake ssh exits 255) and return a warning, got \"\"")
	}

	argv, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("fake ssh was never invoked (no argv recorded): %v", err)
	}
	if !strings.Contains(string(argv), "BatchMode=yes") {
		t.Fatalf("fetch did not pass BatchMode=yes to ssh; argv was:\n%s", argv)
	}
}

// TestNonInteractiveFetchEnv_PreservesUserSSHCommand pins the env contract:
// git's own prompts are disabled, ssh runs in BatchMode, and a user-provided
// GIT_SSH_COMMAND wrapper is preserved (BatchMode appended, not replaced).
func TestNonInteractiveFetchEnv_PreservesUserSSHCommand(t *testing.T) {
	t.Setenv("GIT_SSH_COMMAND", "/usr/bin/my-ssh -i /keys/id")

	var sshCmd, termPrompt string
	for _, kv := range nonInteractiveFetchEnv() {
		if v, ok := strings.CutPrefix(kv, "GIT_SSH_COMMAND="); ok {
			sshCmd = v
		}
		if v, ok := strings.CutPrefix(kv, "GIT_TERMINAL_PROMPT="); ok {
			termPrompt = v
		}
	}

	if termPrompt != "0" {
		t.Fatalf("GIT_TERMINAL_PROMPT = %q, want \"0\"", termPrompt)
	}
	if !strings.HasPrefix(sshCmd, "/usr/bin/my-ssh -i /keys/id") {
		t.Fatalf("user GIT_SSH_COMMAND not preserved: %q", sshCmd)
	}
	if !strings.Contains(sshCmd, "BatchMode=yes") {
		t.Fatalf("GIT_SSH_COMMAND missing BatchMode=yes: %q", sshCmd)
	}
}
