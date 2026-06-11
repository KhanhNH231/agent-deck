package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

func initWorktreeTargetRepo(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "myrepo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-c", "init.defaultBranch=main", "init"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestResolveWorktreeTargetUsesWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	confDir := filepath.Join(home, ".agent-deck")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "config.toml"),
		[]byte("[workspace]\nroot = \"~/ws\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	session.ClearUserConfigCache()
	t.Cleanup(session.ClearUserConfigCache)

	repo := initWorktreeTargetRepo(t)

	wtPath, repoRoot, fallback, errMsg := resolveWorktreeTarget(repo, "feat/login", true)
	if errMsg != "" || fallback {
		t.Fatalf("unexpected errMsg=%q fallback=%v", errMsg, fallback)
	}
	if repoRoot != repo {
		t.Fatalf("repoRoot = %q, want %q", repoRoot, repo)
	}
	want := filepath.Join(home, "ws", "feat-login", "worktrees", "myrepo")
	if wtPath != want {
		t.Fatalf("wtPath = %q, want %q", wtPath, want)
	}
}

func TestResolveWorktreeTargetLegacyWhenWorkspaceAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	session.ClearUserConfigCache()
	t.Cleanup(session.ClearUserConfigCache)

	repo := initWorktreeTargetRepo(t)

	wtPath, _, _, errMsg := resolveWorktreeTarget(repo, "feat/login", true)
	if errMsg != "" {
		t.Fatalf("unexpected errMsg=%q", errMsg)
	}
	// Legacy default location is "subdirectory": <repo>/.worktrees/<branch>.
	want := filepath.Join(repo, ".worktrees", "feat-login")
	if wtPath != want {
		t.Fatalf("wtPath = %q, want %q (legacy fallback)", wtPath, want)
	}
}
