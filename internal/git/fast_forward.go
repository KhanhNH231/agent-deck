package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FFOutcome classifies one repo's fast-forward attempt.
type FFOutcome string

const (
	FFUpdated     FFOutcome = "updated"          // ff applied; OldTip/NewTip set
	FFUpToDate    FFOutcome = "up-to-date"        // local already at upstream tip
	FFDirty       FFOutcome = "dirty-skip"        // uncommitted changes — untouched
	FFDiverged    FFOutcome = "diverged-skip"     // local ahead or histories split
	FFNoUpstream  FFOutcome = "no-upstream"       // branch has no remote counterpart
	FFFetchFailed FFOutcome = "fetch-failed"      // offline/unreachable — untouched
	FFMissing     FFOutcome = "missing-worktree"  // worktree dir absent (used by Task 2)
)

// FFResult carries the outcome of a FastForwardWorktree call.
type FFResult struct {
	Outcome FFOutcome
	OldTip  string // short SHA, set when Outcome == FFUpdated
	NewTip  string // short SHA, set when Outcome == FFUpdated
	Detail  string // human-readable extra (error text on fetch-failed)
}

// FastForwardWorktree fetches the upstream of the branch checked out at
// worktreePath and fast-forwards it iff the worktree is clean and the local
// branch is strictly behind. Never rewrites history, never touches a dirty
// tree. (branch-features spec E4)
//
// Decision order:
//  1. Resolve checked-out branch → none or detached HEAD → FFNoUpstream.
//  2. Upstream lookup via branchUpstreamRef → none → FFNoUpstream.
//  3. Fetch upstream via fetchRemoteShapedRef → warning → FFFetchFailed.
//  4. Dirty check via HasUncommittedChanges → dirty → FFDirty.
//  5. rev-list --left-right --count branch...upstream:
//     ahead > 0 → FFDiverged (regardless of behind);
//     behind == 0 → FFUpToDate;
//     behind > 0 → git merge --ff-only → FFUpdated (OldTip/NewTip set).
//
// error is non-nil only for plumbing failures (rev-parse/rev-list unparsable,
// merge unexpected failure). Skip outcomes are never errors.
func FastForwardWorktree(worktreePath string) (FFResult, error) {
	// Step 1: resolve checked-out branch.
	branch, err := GetCurrentBranch(worktreePath)
	if err != nil || branch == "HEAD" {
		// Detached HEAD or error → no upstream possible.
		return FFResult{Outcome: FFNoUpstream}, nil
	}

	// Step 2: upstream lookup.
	upstream, ok := branchUpstreamRef(worktreePath, branch)
	if !ok {
		return FFResult{Outcome: FFNoUpstream}, nil
	}

	// Step 3: fetch upstream (best-effort; warning means offline/unreachable).
	if warning := fetchRemoteShapedRef(worktreePath, upstream); warning != "" {
		return FFResult{Outcome: FFFetchFailed, Detail: warning}, nil
	}

	// Step 4: dirty check.
	dirty, err := HasUncommittedChanges(worktreePath)
	if err != nil {
		return FFResult{}, fmt.Errorf("dirty check failed: %w", err)
	}
	if dirty {
		return FFResult{Outcome: FFDirty}, nil
	}

	// Step 5: count ahead/behind via rev-list.
	ahead, behind, err := revListAheadBehind(worktreePath, branch, upstream)
	if err != nil {
		return FFResult{}, fmt.Errorf("rev-list failed: %w", err)
	}

	if ahead > 0 {
		return FFResult{Outcome: FFDiverged}, nil
	}
	if behind == 0 {
		return FFResult{Outcome: FFUpToDate}, nil
	}

	// Strictly behind — fast-forward.
	oldTip, err := shortSHA(worktreePath, "HEAD")
	if err != nil {
		return FFResult{}, fmt.Errorf("rev-parse old tip: %w", err)
	}

	mergeCmd := exec.Command("git", "-C", worktreePath, "merge", "--ff-only", upstream)
	if out, mergeErr := mergeCmd.CombinedOutput(); mergeErr != nil {
		return FFResult{}, fmt.Errorf("merge --ff-only failed: %s: %w", strings.TrimSpace(string(out)), mergeErr)
	}

	newTip, err := shortSHA(worktreePath, "HEAD")
	if err != nil {
		return FFResult{}, fmt.Errorf("rev-parse new tip: %w", err)
	}

	return FFResult{
		Outcome: FFUpdated,
		OldTip:  oldTip,
		NewTip:  newTip,
	}, nil
}

// revListAheadBehind returns (ahead, behind) commit counts for branch vs upstream
// using git rev-list --left-right --count branch...upstream.
func revListAheadBehind(repoDir, branch, upstream string) (ahead, behind int, err error) {
	cmd := exec.Command("git", "-C", repoDir, "rev-list", "--left-right", "--count",
		branch+"..."+upstream)
	out, cmdErr := cmd.Output()
	if cmdErr != nil {
		return 0, 0, fmt.Errorf("rev-list: %w", cmdErr)
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("rev-list unexpected output: %q", strings.TrimSpace(string(out)))
	}
	ahead, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse ahead count %q: %w", fields[0], err)
	}
	behind, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse behind count %q: %w", fields[1], err)
	}
	return ahead, behind, nil
}

// shortSHA returns the abbreviated commit SHA for ref in the repo at repoDir.
func shortSHA(repoDir, ref string) (string, error) {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "--short", ref)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
