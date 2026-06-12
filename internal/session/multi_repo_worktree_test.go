package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateMultiRepoWorktrees_BothReposGetWorktreeWithInclude(t *testing.T) {
	repoA := initTestGitRepo(t)
	repoB := initTestGitRepo(t)

	// Both repos have a .worktreeinclude that references a gitignored .env file
	for _, repo := range []string{repoA, repoB} {
		writeTestFile(t, filepath.Join(repo, ".gitignore"), ".env\n")
		testGitAdd(t, repo, ".gitignore")
		writeTestFile(t, filepath.Join(repo, ".env"), "SECRET="+filepath.Base(repo))
		writeTestFile(t, filepath.Join(repo, ".worktreeinclude"), ".env\n")
		testGitAdd(t, repo, ".worktreeinclude")
		testGitCommit(t, repo, "add worktreeinclude")
	}

	parentDir := t.TempDir()
	branch := "test-branch"

	result := CreateMultiRepoWorktrees([]string{repoA, repoB}, parentDir, UniformBranches([]string{repoA, repoB}, branch), 0)

	// Both mapped paths exist and are real directories (not symlinks)
	require.Len(t, result.MappedPaths, 2)
	for _, mp := range result.MappedPaths {
		info, err := os.Lstat(mp)
		require.NoError(t, err)
		assert.True(t, info.IsDir(), "expected real directory, not symlink")
		assert.Zero(t, info.Mode()&os.ModeSymlink)
	}

	// .env from .worktreeinclude landed in both worktrees
	envA, err := os.ReadFile(filepath.Join(result.MappedPaths[0], ".env"))
	require.NoError(t, err)
	assert.Equal(t, "SECRET="+filepath.Base(repoA), string(envA))

	envB, err := os.ReadFile(filepath.Join(result.MappedPaths[1], ".env"))
	require.NoError(t, err)
	assert.Equal(t, "SECRET="+filepath.Base(repoB), string(envB))

	// Worktrees metadata is correct
	require.Len(t, result.Worktrees, 2)
	assert.Equal(t, repoA, result.Worktrees[0].OriginalPath)
	assert.Equal(t, result.MappedPaths[0], result.Worktrees[0].WorktreePath)
	assert.Equal(t, branch, result.Worktrees[0].Branch)
	assert.Equal(t, repoB, result.Worktrees[1].OriginalPath)
	assert.Equal(t, result.MappedPaths[1], result.Worktrees[1].WorktreePath)
	assert.Equal(t, branch, result.Worktrees[1].Branch)

	// No warnings
	assert.Empty(t, result.Warnings)
}

func TestCreateMultiRepoWorktrees_WorktreeCreationFailureIsFatalNoSymlink(t *testing.T) {
	repo := initTestGitRepo(t)

	parentDir := t.TempDir()
	branch := "test-branch"

	// First call succeeds — creates the branch+worktree
	result1 := CreateMultiRepoWorktrees([]string{repo}, parentDir, UniformBranches([]string{repo}, branch), 0)
	require.NoError(t, result1.Err)
	require.Len(t, result1.Worktrees, 1)
	require.Empty(t, result1.Warnings)

	// Second call with same branch fails (branch already checked out). Must be
	// fatal — never symlink the live repo into the session dir.
	parentDir2 := t.TempDir()
	result2 := CreateMultiRepoWorktrees([]string{repo}, parentDir2, UniformBranches([]string{repo}, branch), 0)

	require.Error(t, result2.Err)
	assert.Contains(t, result2.Err.Error(), repo)

	// No symlink (no entry at all) was left behind for the failed repo.
	_, statErr := os.Lstat(filepath.Join(parentDir2, filepath.Base(repo)))
	assert.True(t, os.IsNotExist(statErr), "no entry should be created for the failed repo")
}

func TestCreateMultiRepoWorktrees_FatalFailureRollsBackEarlierWorktree(t *testing.T) {
	repoA := initTestGitRepo(t)
	repoB := initTestGitRepo(t)
	branch := "test-branch"

	// Pre-occupy repoB's branch so the second repo in the list fails.
	occupied := t.TempDir()
	pre := CreateMultiRepoWorktrees([]string{repoB}, occupied, UniformBranches([]string{repoB}, branch), 0)
	require.NoError(t, pre.Err)

	// repoA succeeds, repoB fails → whole result is fatal and repoA's worktree
	// must be rolled back (no orphaned registration in repoA).
	parentDir := t.TempDir()
	result := CreateMultiRepoWorktrees([]string{repoA, repoB}, parentDir, UniformBranches([]string{repoA, repoB}, branch), 0)

	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), repoB)

	// repoA's worktree was unregistered (only the pre-existing one remains in repoB).
	out, err := exec.Command("git", "-C", repoA, "worktree", "list").CombinedOutput()
	require.NoError(t, err, "git worktree list: %s", out)
	assert.NotContains(t, string(out), parentDir, "repoA worktree should be rolled back")
}

func TestCreateMultiRepoWorktrees_NonGitPathGetsSymlinked(t *testing.T) {
	repo := initTestGitRepo(t)
	writeTestFile(t, filepath.Join(repo, ".gitignore"), ".env\n")
	testGitAdd(t, repo, ".gitignore")
	writeTestFile(t, filepath.Join(repo, ".env"), "SECRET=repo")
	writeTestFile(t, filepath.Join(repo, ".worktreeinclude"), ".env\n")
	testGitAdd(t, repo, ".worktreeinclude")
	testGitCommit(t, repo, "setup")

	nonGitDir := t.TempDir()
	writeTestFile(t, filepath.Join(nonGitDir, "data.txt"), "hello")

	parentDir := t.TempDir()

	result := CreateMultiRepoWorktrees([]string{repo, nonGitDir}, parentDir, UniformBranches([]string{repo}, "test-branch"), 0)

	require.Len(t, result.MappedPaths, 2)

	// First path: real worktree
	info, err := os.Lstat(result.MappedPaths[0])
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Zero(t, info.Mode()&os.ModeSymlink)

	// Second path: symlink to original non-git dir
	info, err = os.Lstat(result.MappedPaths[1])
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	target, err := os.Readlink(result.MappedPaths[1])
	require.NoError(t, err)
	assert.Equal(t, nonGitDir, target)

	// Only the git repo appears in Worktrees
	require.Len(t, result.Worktrees, 1)
	assert.Equal(t, repo, result.Worktrees[0].OriginalPath)

	assert.Empty(t, result.Warnings)
}

// --- E2: per-repo branch map tests ---

func TestCreateMultiRepoWorktrees_DifferentBranchesPerRepo(t *testing.T) {
	repoA := initTestGitRepo(t)
	repoB := initTestGitRepo(t)
	parentDir := t.TempDir()

	branches := MultiRepoBranches{
		repoA: "branch-alpha",
		repoB: "branch-beta",
	}
	result := CreateMultiRepoWorktrees([]string{repoA, repoB}, parentDir, branches, 0)

	require.NoError(t, result.Err)
	require.Len(t, result.Worktrees, 2)

	// Each worktree must be on its own branch.
	for _, wt := range result.Worktrees {
		gotBranch := strings.TrimSpace(gitBranchHead(t, wt.WorktreePath))
		wantBranch := branches[wt.OriginalPath]
		if gotBranch != wantBranch {
			t.Errorf("repo %s: HEAD branch = %q, want %q", wt.OriginalPath, gotBranch, wantBranch)
		}
		if wt.Branch != wantBranch {
			t.Errorf("repo %s: MultiRepoWorktree.Branch = %q, want %q", wt.OriginalPath, wt.Branch, wantBranch)
		}
	}
}

func TestCreateMultiRepoWorktrees_MissingBranchKeyFails(t *testing.T) {
	repoA := initTestGitRepo(t)
	repoB := initTestGitRepo(t)
	parentDir := t.TempDir()

	// repoB is absent from the map — must be a fatal error.
	branches := MultiRepoBranches{
		repoA: "branch-alpha",
	}
	result := CreateMultiRepoWorktrees([]string{repoA, repoB}, parentDir, branches, 0)

	require.Error(t, result.Err, "expected fatal error when branch key is missing")
	assert.Contains(t, result.Err.Error(), repoB, "error should name the missing repo path")

	// Rollback: repoA's worktree must be unregistered.
	out, err := exec.Command("git", "-C", repoA, "worktree", "list").CombinedOutput()
	require.NoError(t, err, "git worktree list: %s", out)
	assert.NotContains(t, string(out), parentDir, "repoA worktree should have been rolled back")
}

func TestUniformBranches_AllPathsOnSameBranch(t *testing.T) {
	paths := []string{"/a/repo", "/b/repo"}
	branches := UniformBranches(paths, "feat/x")
	for _, p := range paths {
		got, ok := branches[p]
		require.True(t, ok, "path %s missing from UniformBranches result", p)
		assert.Equal(t, "feat/x", got)
	}
}

// --- E2: Reconcile per-repo branch tests ---

func TestReconcileMultiRepoWorktrees_AddedRepoOnOwnBranch(t *testing.T) {
	repoA := initFeatureTestRepo(t, "repo-a")
	repoB := initFeatureTestRepo(t, "repo-b")
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	// Seed: repoA already worktree'd on "feat-a".
	first := CreateMultiRepoWorktrees([]string{repoA}, tempDir, UniformBranches([]string{repoA}, "feat-a"), time.Minute)
	require.NoError(t, first.Err)

	// Reconcile: add repoB on a DIFFERENT branch "feat-b".
	branches := MultiRepoBranches{
		repoA: "feat-a", // kept — branch lookup unused for kept worktrees
		repoB: "feat-b",
	}
	res := ReconcileMultiRepoWorktrees(tempDir, branches, first.Worktrees, []string{repoA, repoB}, time.Minute)
	require.NoError(t, res.Err)
	require.Len(t, res.Worktrees, 2)

	// Find the new repoB worktree and assert its branch.
	var repoBWorktree *MultiRepoWorktree
	for i := range res.Worktrees {
		if res.Worktrees[i].OriginalPath == repoB {
			repoBWorktree = &res.Worktrees[i]
		}
	}
	require.NotNil(t, repoBWorktree, "repoB missing from reconciled worktrees")

	gotBranch := strings.TrimSpace(gitBranchHead(t, repoBWorktree.WorktreePath))
	assert.Equal(t, "feat-b", gotBranch, "repoB worktree should be on feat-b")
	assert.Equal(t, "feat-b", repoBWorktree.Branch)

	// repoA's worktree is untouched (kept worktree — same path).
	assert.Equal(t, first.Worktrees[0].WorktreePath, res.Worktrees[0].WorktreePath)
}

func TestReconcileMultiRepoWorktrees_MissingKeyAbortsBeforeRemoval(t *testing.T) {
	repoA := initFeatureTestRepo(t, "repo-a")
	repoB := initFeatureTestRepo(t, "repo-b")
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	// Seed: repoA worktree'd on "feat-a".
	first := CreateMultiRepoWorktrees([]string{repoA}, tempDir, UniformBranches([]string{repoA}, "feat-a"), time.Minute)
	require.NoError(t, first.Err)
	aWorktree := first.Worktrees[0].WorktreePath

	// New set drops repoA and adds repoB — but the branch map lacks repoB.
	// Validation must fail BEFORE repoA's worktree is removed: a half-applied
	// reconcile (removed but nothing added) is inconsistent state.
	branches := MultiRepoBranches{} // repoB missing
	res := ReconcileMultiRepoWorktrees(tempDir, branches, first.Worktrees, []string{repoB}, time.Minute)

	require.Error(t, res.Err, "expected fatal error for missing branch key")
	assert.Contains(t, res.Err.Error(), repoB, "error should name the missing repo path")

	// repoA's worktree must be untouched — still on disk and still registered.
	_, statErr := os.Stat(aWorktree)
	assert.NoError(t, statErr, "repoA worktree should still exist on disk")
	out, gitErr := exec.Command("git", "-C", repoA, "worktree", "list").CombinedOutput()
	require.NoError(t, gitErr, "git worktree list: %s", out)
	assert.Contains(t, string(out), aWorktree, "repoA worktree should still be registered")
}

func gitBranchHead(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	require.NoError(t, err, "git rev-parse --abbrev-ref HEAD in %s: %s", dir, out)
	return strings.TrimSpace(string(out))
}

// --- test helpers ---

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	writeTestFile(t, filepath.Join(dir, "README.md"), "# test")
	testGitAdd(t, dir, ".")
	testGitCommit(t, dir, "init")
	return dir
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func testGitAdd(t *testing.T, dir, path string) {
	t.Helper()
	cmd := exec.Command("git", "add", path)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git add %s: %s", path, out)
}

func testGitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git commit: %s", out)
}
