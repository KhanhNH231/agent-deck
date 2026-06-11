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

// ReconcileMultiRepoWorktrees brings a worktree session's repo set in line
// with newPaths without touching surviving worktrees (uncommitted work in
// them is preserved):
//   - paths that already have a worktree keep it as-is
//   - added git repos get a REAL worktree (fail-loud, like creation — never a
//     live-repo symlink)
//   - added non-git dirs are symlinked (live view, matching creation)
//   - repos dropped from the set have their worktree removed and unregistered
//     (refused while dirty — force-remove is the caller's explicit decision)
//
// On a fatal error only the worktrees newly created by THIS call are rolled
// back; pre-existing ones are left untouched.
func ReconcileMultiRepoWorktrees(parentDir, branch string, existing []MultiRepoWorktree, newPaths []string, setupTimeout time.Duration) MultiRepoWorktreeResult {
	var result MultiRepoWorktreeResult

	existingByOriginal := make(map[string]MultiRepoWorktree, len(existing))
	for _, wt := range existing {
		existingByOriginal[wt.OriginalPath] = wt
	}
	keep := make(map[string]bool, len(newPaths))
	for _, p := range newPaths {
		keep[p] = true
	}

	// Remove worktrees for repos dropped from the set first, so their
	// dirnames are free for re-use. Dirty worktrees abort the reconcile
	// before anything else changes.
	for _, wt := range existing {
		if keep[wt.OriginalPath] {
			continue
		}
		if err := git.RemoveWorktree(wt.RepoRoot, wt.WorktreePath, false); err != nil {
			result.Err = fmt.Errorf("remove worktree %s: %w", wt.OriginalPath, err)
			return result
		}
	}

	var created []MultiRepoWorktree
	dirnames := DeduplicateDirnames(newPaths)
	for i, p := range newPaths {
		if wt, ok := existingByOriginal[p]; ok {
			result.Worktrees = append(result.Worktrees, wt)
			result.MappedPaths = append(result.MappedPaths, wt.WorktreePath)
			continue
		}
		wtPath := filepath.Join(parentDir, dirnames[i])

		if git.IsGitRepoOrBareProjectRoot(p) {
			repoRoot, rootErr := git.GetWorktreeBaseRoot(p)
			if rootErr != nil {
				result.Err = fmt.Errorf("cannot isolate %s: resolve repo root: %w", p, rootErr)
				rollbackMultiRepoWorktrees(created)
				return result
			}
			var buf bytes.Buffer
			setupErr, err := git.CreateWorktreeWithSetup(repoRoot, wtPath, branch, &buf, &buf, setupTimeout)
			if err != nil {
				result.Err = fmt.Errorf("cannot isolate %s: create worktree: %w", p, err)
				rollbackMultiRepoWorktrees(created)
				return result
			}
			if setupErr != nil {
				result.Warnings = append(result.Warnings, "worktree_setup_fail: "+p+": "+setupErr.Error())
			}
			wt := MultiRepoWorktree{
				OriginalPath: p,
				WorktreePath: wtPath,
				RepoRoot:     repoRoot,
				Branch:       branch,
			}
			created = append(created, wt)
			result.Worktrees = append(result.Worktrees, wt)
			result.MappedPaths = append(result.MappedPaths, wtPath)
		} else {
			// Non-git dir: refresh the symlink (it may point elsewhere from
			// an earlier path set).
			_ = os.Remove(wtPath)
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
