# Multi-repo worktree: no silent symlink to main repo

**Date:** 2026-06-10
**Branch:** feat/agent-deck-session-ux
**Status:** Approved

## Problem

When a multi-repo session is created **with a worktree branch**,
`session.CreateMultiRepoWorktrees` creates one real git worktree per repo inside
`~/.agent-deck/multi-repo-worktrees/<branch>-<id>/`. But when worktree creation
fails for a git repo, it silently falls back to `os.Symlink(originalPath, wtPath)`:

- `multi_repo_worktree.go:29` — `GetWorktreeBaseRoot` error
- `multi_repo_worktree.go:38` — `CreateWorktreeWithSetup` error

The session then *looks* isolated, but the symlinked entry points at the **live
repo checkout**. Any edit the agent makes flows straight into the main repo,
defeating the purpose of a worktree session. The failure is silent (warning log
only), so the user never knows isolation was lost.

## Decision

For a **git repo**, worktree-creation failure is fatal: abort session creation
with a visible error naming the repo and reason. Never symlink a git repo into a
worktree session.

For a **non-git dir** (`multi_repo_worktree.go:53` else-branch — e.g. a docs or
config folder), keep the symlink. It is not a repo, there is nothing to isolate,
and it is not "the main repo". The session proceeds.

`setupErr` (post-create setup script failure, line 42) stays a non-fatal
warning — the worktree exists and is isolated; the setup script is best-effort.

The multi-repo path **without** a worktree branch (`home.go:9218–9244`) is
unchanged: symlinking live repos there is intended (no isolation requested).

## Changes

### `internal/session/multi_repo_worktree.go`

- Extend `MultiRepoWorktreeResult`:
  - `Err error` — set when a git repo cannot be isolated. Non-nil means the
    caller must abort.
  - `Created []MultiRepoWorktree` — worktrees actually created so far, for
    rollback. (May reuse existing `Worktrees`; see Implementation note.)
- In the git-repo branch: on `GetWorktreeBaseRoot` error or
  `CreateWorktreeWithSetup` error, set `result.Err` (wrapping repo path +
  reason), roll back, and `return` immediately — do **not** `os.Symlink`.
- Rollback: before returning with `Err`, iterate worktrees created so far and
  call `git.RemoveWorktree(repoRoot, wtPath, true)` for each, so a partial run
  leaves no orphaned `.git/worktrees/<id>` registration.
- Non-git else-branch: unchanged (`os.Symlink`).

### `internal/ui/home.go` (caller ~9210)

After `wtResult := session.CreateMultiRepoWorktrees(...)`:

```go
if wtResult.Err != nil {
    _ = os.RemoveAll(parentDir)
    return sessionCreatedMsg{
        err:    fmt.Errorf("multi-repo worktree: %w", wtResult.Err),
        tempID: tempID,
    }
}
```

Placed before the existing warning loop / mapped-path assignment so we abort
before mutating `inst`.

## Edge cases

| Case | Behavior |
|------|----------|
| Repo A worktree OK, repo B fails | Roll back A's worktree, remove parentDir, abort with B named |
| Branch already exists / dirty tree | `CreateWorktreeWithSetup` errors → abort |
| Non-git docs dir, all repos OK | Docs dir symlinked, session proceeds |
| Non-git docs dir + a git repo fails | Whole session aborts (git failure dominates) |
| `setupErr` only (worktree created) | Warning, session proceeds |
| All worktrees succeed | Unchanged from today |

## Tests (`multi_repo_worktree_test.go`)

1. Git repo whose worktree creation fails → `result.Err != nil`, no symlink at
   `wtPath`, and any previously-created worktree is rolled back.
2. Mixed list, all git repos succeed + one non-git dir → non-git dir is a
   symlink, git entries are real worktrees, `Err == nil`.
3. All-success multi-repo path unchanged (regression guard).

Caller-side abort (`home.go`) is covered by the unit-level `Err` contract; a
full UI test is out of scope (no existing harness for this create path).

## Files

- `internal/session/multi_repo_worktree.go`
- `internal/ui/home.go`
- `internal/session/multi_repo_worktree_test.go`
