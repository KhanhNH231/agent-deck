package git

// Tests E1 (branch-features spec): worktree creation fetches remote-shaped
// refs before branching, and degrades to a warning when the remote is
// unreachable. Repo topology mirrors issue973_worker_spawn_fresh_main_test.go:
// bare origin + two clones; cloneB advances origin after cloneA's initial fetch,
// so cloneA's remote-tracking ref is genuinely stale.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupStaleTrackingReposE1 creates a bare origin + cloneA + cloneB.
// cloneA has a stale remote-tracking ref: origin advanced past cloneA's last fetch.
// Returns (originDir, cloneA, cloneB).
func setupStaleTrackingReposE1(t *testing.T) (origin, cloneA, cloneB string) {
	t.Helper()
	tmp := t.TempDir()

	origin = filepath.Join(tmp, "origin.git")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatalf("mkdir origin: %v", err)
	}
	mustGitE1(t, origin, "init", "--bare", "-b", "main")

	cloneA = filepath.Join(tmp, "cloneA")
	if err := os.MkdirAll(cloneA, 0o755); err != nil {
		t.Fatalf("mkdir cloneA: %v", err)
	}
	mustGitE1(t, cloneA, "init", "-b", "main")
	mustGitE1(t, cloneA, "config", "user.email", "test@test.com")
	mustGitE1(t, cloneA, "config", "user.name", "Test User")
	mustGitE1(t, cloneA, "remote", "add", "origin", origin)

	// Seed commit so cloneA can push.
	if err := os.WriteFile(filepath.Join(cloneA, "README.md"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	mustGitE1(t, cloneA, "add", ".")
	mustGitE1(t, cloneA, "commit", "-m", "seed")
	mustGitE1(t, cloneA, "push", "-u", "origin", "main")

	// cloneB is a fresh clone — used to advance origin after cloneA's last fetch.
	// git clone creates the destination dir itself; do NOT pre-create it.
	cloneB = filepath.Join(tmp, "cloneB")
	mustGitE1(t, tmp, "clone", origin, cloneB)
	mustGitE1(t, cloneB, "config", "user.email", "test@test.com")
	mustGitE1(t, cloneB, "config", "user.name", "Test User")

	return origin, cloneA, cloneB
}

// advanceBranchE1 commits a new file on branch in repo and pushes. Returns the new tip SHA.
func advanceBranchE1(t *testing.T, repo, branch, filename string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, filename), []byte("advance"), 0o644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
	mustGitE1(t, repo, "add", ".")
	mustGitE1(t, repo, "commit", "-m", "advance "+filename)
	mustGitE1(t, repo, "push", "origin", branch)
	return gitOutputE1(t, repo, "rev-parse", "HEAD")
}

// mustGitE1 runs git with args in dir and fatals on error.
func mustGitE1(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, strings.TrimSpace(string(out)))
	}
}

// gitOutputE1 runs git with args in dir and returns trimmed stdout. Fatals on error.
func gitOutputE1(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out))
}

// TestCreateWorktreeAtStartPoint_FetchesRemoteShapedStartPoint verifies that
// CreateWorktreeAtStartPoint fetches the remote ref before branching, so the
// new worktree lands on the wire tip — not cloneA's stale remote-tracking ref.
func TestCreateWorktreeAtStartPoint_FetchesRemoteShapedStartPoint(t *testing.T) {
	origin, cloneA, cloneB := setupStaleTrackingReposE1(t)
	_ = origin

	// Advance origin via cloneB AFTER cloneA's last fetch.
	advancedTip := advanceBranchE1(t, cloneB, "main", "cloneB-advance.txt")

	// Confirm cloneA is genuinely stale.
	staleRef := gitOutputE1(t, cloneA, "rev-parse", "origin/main")
	if staleRef == advancedTip {
		t.Fatal("test setup invalid: cloneA already has the advanced tip")
	}

	wt := t.TempDir()
	created, warning, err := CreateWorktreeAtStartPoint(cloneA, wt, "e1-feature", "origin/main")
	if err != nil {
		t.Fatalf("CreateWorktreeAtStartPoint: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}

	head, err := HeadCommit(wt)
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if head != advancedTip {
		t.Fatalf("worktree HEAD = %s, want advancedTip = %s (stale = %s); fetch did not update the tracking ref before branch", head, advancedTip, staleRef)
	}
}

// TestCreateWorktreeAtStartPoint_UnreachableRemoteWarnsAndProceeds verifies
// offline degradation: when the remote is unreachable, CreateWorktreeAtStartPoint
// returns a non-empty warning but still creates the worktree from the stale ref.
func TestCreateWorktreeAtStartPoint_UnreachableRemoteWarnsAndProceeds(t *testing.T) {
	_, cloneA, cloneB := setupStaleTrackingReposE1(t)

	// Advance so cloneA has a known stale tip.
	_ = advanceBranchE1(t, cloneB, "main", "cloneB-advance2.txt")
	staleTip := gitOutputE1(t, cloneA, "rev-parse", "origin/main")

	// Break the remote.
	mustGitE1(t, cloneA, "remote", "set-url", "origin", "/nonexistent/path/that/does/not/exist.git")

	wt := filepath.Join(t.TempDir(), "offline-wt")
	created, warning, err := CreateWorktreeAtStartPoint(cloneA, wt, "e1-offline", "origin/main")
	if err != nil {
		t.Fatalf("expected no error on unreachable remote, got: %v", err)
	}
	if !created {
		t.Fatal("expected created=true even when offline")
	}
	if warning == "" {
		t.Fatal("expected non-empty warning for unreachable remote")
	}
	if !strings.Contains(warning, "origin") {
		t.Fatalf("warning should mention 'origin', got %q", warning)
	}

	// Worktree dir must exist.
	if _, statErr := os.Stat(wt); statErr != nil {
		t.Fatalf("worktree dir should exist: %v", statErr)
	}

	head, err := HeadCommit(wt)
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if head != staleTip {
		t.Fatalf("offline: worktree HEAD = %s, want stale tip = %s", head, staleTip)
	}
}

// TestCreateWorktreeAtStartPoint_NonRemoteStartPointNoFetchNoWarning verifies
// that a raw SHA start point bypasses fetch entirely — no warning, no error.
func TestCreateWorktreeAtStartPoint_NonRemoteStartPointNoFetchNoWarning(t *testing.T) {
	_, cloneA, _ := setupStaleTrackingReposE1(t)

	sha := gitOutputE1(t, cloneA, "rev-parse", "HEAD")

	wt := t.TempDir()
	created, warning, err := CreateWorktreeAtStartPoint(cloneA, wt, "e1-sha-branch", sha)
	if err != nil {
		t.Fatalf("CreateWorktreeAtStartPoint: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if warning != "" {
		t.Fatalf("expected no warning for raw SHA, got %q", warning)
	}

	head, err := HeadCommit(wt)
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if head != sha {
		t.Fatalf("HEAD = %s, want %s", head, sha)
	}
}
