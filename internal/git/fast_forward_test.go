package git

// Tests for FastForwardWorktree (branch-features spec E4).
// Uses the E1 fixture helpers (setupStaleTrackingReposE1, advanceBranchE1,
// mustGitE1, gitOutputE1) defined in branch_freshen_test.go.

import (
	"os"
	"path/filepath"
	"testing"
)

// setupFFWorktree creates a minimal repo topology for fast-forward tests:
//   - bare origin with a seed commit on "main"
//   - cloneA (the "local" repo) with a checked-out worktree on "main" tracking origin/main
//   - cloneB (the "remote pusher") for advancing origin
//
// Returns (worktreeDir, cloneADir, cloneBDir).
// The returned worktreeDir IS the working directory already checked out on "main".
func setupFFWorktree(t *testing.T) (wtDir, cloneA, cloneB string) {
	t.Helper()
	origin, cloneA, cloneB := setupStaleTrackingReposE1(t)
	_ = origin

	// cloneA currently has main checked out; use it directly as the worktree
	// (it's already tracking origin/main via push -u).
	wtDir = cloneA
	return wtDir, cloneA, cloneB
}

// TestFastForwardWorktree_BehindOnly verifies that a clean worktree that is
// strictly behind its upstream is fast-forwarded: FFUpdated, OldTip≠NewTip,
// and the remote's new file appears on disk.
func TestFastForwardWorktree_BehindOnly(t *testing.T) {
	wtDir, _, cloneB := setupFFWorktree(t)

	oldTip := gitOutputE1(t, wtDir, "rev-parse", "--short", "HEAD")

	// Advance origin via cloneB — wtDir is now behind.
	advanceBranchE1(t, cloneB, "main", "ff-behind-file.txt")

	result, err := FastForwardWorktree(wtDir)
	if err != nil {
		t.Fatalf("FastForwardWorktree: unexpected error: %v", err)
	}
	if result.Outcome != FFUpdated {
		t.Fatalf("outcome = %q, want FFUpdated", result.Outcome)
	}
	if result.OldTip == "" {
		t.Fatal("OldTip must be non-empty after FFUpdated")
	}
	if result.NewTip == "" {
		t.Fatal("NewTip must be non-empty after FFUpdated")
	}
	if result.OldTip == result.NewTip {
		t.Fatalf("OldTip == NewTip (%s): branch did not move", result.OldTip)
	}
	// Sanity: OldTip matches the pre-ff HEAD.
	if result.OldTip != oldTip {
		t.Fatalf("OldTip = %s, want %s", result.OldTip, oldTip)
	}
	// The file pushed by cloneB must exist in the worktree.
	if _, statErr := os.Stat(filepath.Join(wtDir, "ff-behind-file.txt")); statErr != nil {
		t.Fatalf("new file from remote not present in worktree: %v", statErr)
	}
}

// TestFastForwardWorktree_Equal verifies that a worktree already at the
// upstream tip returns FFUpToDate without moving the branch.
func TestFastForwardWorktree_Equal(t *testing.T) {
	wtDir, _, _ := setupFFWorktree(t)

	tipBefore := gitOutputE1(t, wtDir, "rev-parse", "HEAD")

	result, err := FastForwardWorktree(wtDir)
	if err != nil {
		t.Fatalf("FastForwardWorktree: unexpected error: %v", err)
	}
	if result.Outcome != FFUpToDate {
		t.Fatalf("outcome = %q, want FFUpToDate", result.Outcome)
	}

	tipAfter := gitOutputE1(t, wtDir, "rev-parse", "HEAD")
	if tipBefore != tipAfter {
		t.Fatalf("branch moved unexpectedly: %s → %s", tipBefore, tipAfter)
	}
}

// TestFastForwardWorktree_DirtyBehind verifies that a dirty worktree that is
// behind its upstream returns FFDirty, leaves dirty content untouched, and
// does NOT move the branch tip.
func TestFastForwardWorktree_DirtyBehind(t *testing.T) {
	wtDir, _, cloneB := setupFFWorktree(t)

	tipBefore := gitOutputE1(t, wtDir, "rev-parse", "HEAD")

	// Advance origin so wtDir is behind.
	advanceBranchE1(t, cloneB, "main", "ff-dirty-remote.txt")

	// Dirty the worktree with an untracked file.
	dirtyFile := filepath.Join(wtDir, "dirty-work.txt")
	if err := os.WriteFile(dirtyFile, []byte("uncommitted"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	result, err := FastForwardWorktree(wtDir)
	if err != nil {
		t.Fatalf("FastForwardWorktree: unexpected error: %v", err)
	}
	if result.Outcome != FFDirty {
		t.Fatalf("outcome = %q, want FFDirty", result.Outcome)
	}

	// Dirty file must still exist with its original content.
	content, readErr := os.ReadFile(dirtyFile)
	if readErr != nil {
		t.Fatalf("dirty file disappeared: %v", readErr)
	}
	if string(content) != "uncommitted" {
		t.Fatalf("dirty file content changed: %q", string(content))
	}

	// Branch tip must not have moved.
	tipAfter := gitOutputE1(t, wtDir, "rev-parse", "HEAD")
	if tipBefore != tipAfter {
		t.Fatalf("branch moved on dirty worktree: %s → %s", tipBefore, tipAfter)
	}
}

// TestFastForwardWorktree_Diverged verifies that when the local branch has a
// local commit AND the remote has a commit not in local, the result is
// FFDiverged and the local tip is unchanged.
func TestFastForwardWorktree_Diverged(t *testing.T) {
	wtDir, _, cloneB := setupFFWorktree(t)

	// Advance remote.
	advanceBranchE1(t, cloneB, "main", "ff-diverge-remote.txt")

	// Commit locally — creating divergence.
	localFile := filepath.Join(wtDir, "ff-diverge-local.txt")
	if err := os.WriteFile(localFile, []byte("local diverge"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	mustGitE1(t, wtDir, "add", ".")
	mustGitE1(t, wtDir, "commit", "-m", "local diverge commit")

	localTip := gitOutputE1(t, wtDir, "rev-parse", "HEAD")

	result, err := FastForwardWorktree(wtDir)
	if err != nil {
		t.Fatalf("FastForwardWorktree: unexpected error: %v", err)
	}
	if result.Outcome != FFDiverged {
		t.Fatalf("outcome = %q, want FFDiverged", result.Outcome)
	}

	// Local tip must be untouched.
	tipAfter := gitOutputE1(t, wtDir, "rev-parse", "HEAD")
	if localTip != tipAfter {
		t.Fatalf("local tip changed on diverged: %s → %s", localTip, tipAfter)
	}
}

// TestFastForwardWorktree_NoUpstream verifies that a branch with no remote
// counterpart returns FFNoUpstream without error.
func TestFastForwardWorktree_NoUpstream(t *testing.T) {
	_, cloneA, _ := setupFFWorktree(t)

	// Create a new local-only branch in a separate worktree directory.
	localBranch := "local-only-no-upstream"
	mustGitE1(t, cloneA, "branch", localBranch)

	// Create a worktree for it.
	localWt := filepath.Join(t.TempDir(), "local-wt")
	mustGitE1(t, cloneA, "worktree", "add", localWt, localBranch)

	result, err := FastForwardWorktree(localWt)
	if err != nil {
		t.Fatalf("FastForwardWorktree: unexpected error: %v", err)
	}
	if result.Outcome != FFNoUpstream {
		t.Fatalf("outcome = %q, want FFNoUpstream", result.Outcome)
	}
}

// TestFastForwardWorktree_UnreachableRemote verifies that an unreachable
// remote returns FFFetchFailed with a non-empty Detail and no error return.
func TestFastForwardWorktree_UnreachableRemote(t *testing.T) {
	wtDir, _, _ := setupFFWorktree(t)

	// Break the remote URL.
	mustGitE1(t, wtDir, "remote", "set-url", "origin", "/nonexistent/path/that/does/not/exist.git")

	result, err := FastForwardWorktree(wtDir)
	if err != nil {
		t.Fatalf("FastForwardWorktree: unexpected error: %v", err)
	}
	if result.Outcome != FFFetchFailed {
		t.Fatalf("outcome = %q, want FFFetchFailed", result.Outcome)
	}
	if result.Detail == "" {
		t.Fatal("Detail must be non-empty for FFFetchFailed")
	}
}
