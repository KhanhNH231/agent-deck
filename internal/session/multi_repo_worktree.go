package session

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/git"
)

type MultiRepoWorktreeResult struct {
	MappedPaths []string
	Worktrees   []MultiRepoWorktree
	Warnings    []string
	// Err is set when a git repo could not be isolated into its own worktree.
	// It is fatal: the caller must abort session creation rather than launch a
	// session that silently aliases the live repo. Already-created worktrees are
	// rolled back before this is returned.
	Err error
}

func CreateMultiRepoWorktrees(allPaths []string, parentDir string, branch string, setupTimeout time.Duration) MultiRepoWorktreeResult {
	var result MultiRepoWorktreeResult
	dirnames := DeduplicateDirnames(allPaths)

	for i, p := range allPaths {
		wtPath := filepath.Join(parentDir, dirnames[i])

		if git.IsGitRepoOrBareProjectRoot(p) {
			// A git repo MUST get a real, isolated worktree. Failure here is
			// fatal: silently symlinking the live checkout would let the agent's
			// edits flow straight into the main repo while the session looks
			// isolated. Roll back what we created and let the caller abort.
			repoRoot, rootErr := git.GetWorktreeBaseRoot(p)
			if rootErr != nil {
				result.Err = fmt.Errorf("cannot isolate %s: resolve repo root: %w", p, rootErr)
				rollbackMultiRepoWorktrees(result.Worktrees)
				return result
			}

			var buf bytes.Buffer
			setupErr, err := git.CreateWorktreeWithSetup(repoRoot, wtPath, branch, &buf, &buf, setupTimeout)
			if err != nil {
				result.Err = fmt.Errorf("cannot isolate %s: create worktree: %w", p, err)
				rollbackMultiRepoWorktrees(result.Worktrees)
				return result
			}
			if setupErr != nil {
				result.Warnings = append(result.Warnings, "worktree_setup_fail: "+p+": "+setupErr.Error())
			}

			result.Worktrees = append(result.Worktrees, MultiRepoWorktree{
				OriginalPath: p,
				WorktreePath: wtPath,
				RepoRoot:     repoRoot,
				Branch:       branch,
			})
			result.MappedPaths = append(result.MappedPaths, wtPath)
		} else {
			// Non-git dir (docs/config folder): nothing to isolate, not "the main
			// repo". Symlink it so the session sees it live.
			_ = os.Symlink(p, wtPath)
			result.MappedPaths = append(result.MappedPaths, wtPath)
		}
	}

	return result
}

// rollbackMultiRepoWorktrees unregisters worktrees created before a fatal
// failure, so a partially-built multi-repo session leaves no orphaned
// .git/worktrees entries. Best-effort: errors are ignored (the caller also
// removes the parent dir).
func rollbackMultiRepoWorktrees(created []MultiRepoWorktree) {
	for _, wt := range created {
		_ = git.RemoveWorktree(wt.RepoRoot, wt.WorktreePath, true)
	}
}
