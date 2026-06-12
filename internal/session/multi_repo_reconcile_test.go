package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestReconcileAddsRealWorktreeNotSymlink(t *testing.T) {
	repoA := initFeatureTestRepo(t, "repo-a")
	repoB := initFeatureTestRepo(t, "repo-b")
	tempDir := t.TempDir()

	// Existing state: repo-a already has a worktree for the branch.
	first := CreateMultiRepoWorktrees([]string{repoA}, tempDir, UniformBranches([]string{repoA}, "feat-x"), time.Minute)
	if first.Err != nil {
		t.Fatalf("seed worktree: %v", first.Err)
	}
	// Uncommitted work in the existing worktree MUST survive reconcile.
	marker := filepath.Join(first.Worktrees[0].WorktreePath, "wip.txt")
	if err := os.WriteFile(marker, []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := ReconcileMultiRepoWorktrees(tempDir, UniformBranches([]string{repoA, repoB}, "feat-x"), first.Worktrees, []string{repoA, repoB}, time.Minute)
	if res.Err != nil {
		t.Fatalf("reconcile: %v", res.Err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("uncommitted work destroyed by reconcile: %v", err)
	}
	if len(res.Worktrees) != 2 {
		t.Fatalf("worktrees = %d, want 2", len(res.Worktrees))
	}
	// repo-b entry must be a real directory worktree, not a symlink.
	var bPath string
	for _, wt := range res.Worktrees {
		if wt.RepoRoot == repoB {
			bPath = wt.WorktreePath
		}
	}
	if bPath == "" {
		t.Fatal("repo-b missing from reconciled worktrees")
	}
	fi, err := os.Lstat(bPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("repo-b is a symlink to the live repo — landmine regression")
	}
	if got := gitOut(t, repoB, "worktree", "list"); !containsPath(got, bPath) {
		t.Fatalf("repo-b worktree not registered:\n%s", got)
	}
}

func TestReconcileRemovesDroppedRepoWorktree(t *testing.T) {
	repoA := initFeatureTestRepo(t, "repo-a")
	repoB := initFeatureTestRepo(t, "repo-b")
	tempDir := t.TempDir()

	first := CreateMultiRepoWorktrees([]string{repoA, repoB}, tempDir, UniformBranches([]string{repoA, repoB}, "feat-x"), time.Minute)
	if first.Err != nil {
		t.Fatalf("seed: %v", first.Err)
	}

	res := ReconcileMultiRepoWorktrees(tempDir, UniformBranches([]string{repoA}, "feat-x"), first.Worktrees, []string{repoA}, time.Minute)
	if res.Err != nil {
		t.Fatalf("reconcile: %v", res.Err)
	}
	if len(res.Worktrees) != 1 || res.Worktrees[0].RepoRoot != repoA {
		t.Fatalf("worktrees = %+v", res.Worktrees)
	}
	if got := gitOut(t, repoB, "worktree", "list"); containsPath(got, filepath.Join(tempDir, "repo-b")) {
		t.Fatalf("repo-b worktree still registered:\n%s", got)
	}
}

func TestReconcileSymlinksNonGitDirs(t *testing.T) {
	repoA := initFeatureTestRepo(t, "repo-a")
	docs := t.TempDir()
	tempDir := t.TempDir()

	first := CreateMultiRepoWorktrees([]string{repoA}, tempDir, UniformBranches([]string{repoA}, "feat-x"), time.Minute)
	if first.Err != nil {
		t.Fatalf("seed: %v", first.Err)
	}

	res := ReconcileMultiRepoWorktrees(tempDir, UniformBranches([]string{repoA}, "feat-x"), first.Worktrees, []string{repoA, docs}, time.Minute)
	if res.Err != nil {
		t.Fatalf("reconcile: %v", res.Err)
	}
	// The docs entry is mapped and is a symlink (live view is intended for
	// non-git dirs).
	if len(res.MappedPaths) != 2 {
		t.Fatalf("mapped = %v", res.MappedPaths)
	}
	fi, err := os.Lstat(res.MappedPaths[1])
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("non-git dir should be symlinked, got %v", fi.Mode())
	}
}

func containsPath(haystack, p string) bool {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		resolved = p
	}
	return strings.Contains(haystack, p) || strings.Contains(haystack, resolved)
}
