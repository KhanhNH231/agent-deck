# E4: `feature update` (ff-pull) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** `agent-deck feature update <name>` + TUI hotkey: fetch + fast-forward every repo branch in a feature from `origin/<same-branch>`. ff-only, clean-worktree-gated, per-repo outcome report.

**Spec:** E4 section of `docs/superpowers/specs/2026-06-12-branch-features-design.md`. Safety invariants: ff-only, never dirty, diverged → skip.
**Branch:** feat/agent-deck-session-ux

## Task 1: `git.FastForwardWorktree` (TDD, internal/git only)

**Files:** `internal/git/git.go` (or new `internal/git/fast_forward.go` if git.go feels crowded — implementer judgment), tests appended to `internal/git/branch_freshen_test.go` (reuse its E1 fixtures).

```go
// FFOutcome classifies one repo's fast-forward attempt.
type FFOutcome string

const (
	FFUpdated     FFOutcome = "updated"      // ff applied; OldTip/NewTip set
	FFUpToDate    FFOutcome = "up-to-date"
	FFDirty       FFOutcome = "dirty-skip"   // uncommitted changes — untouched
	FFDiverged    FFOutcome = "diverged-skip" // local ahead or histories split
	FFNoUpstream  FFOutcome = "no-upstream"  // branch has no remote counterpart
	FFFetchFailed FFOutcome = "fetch-failed" // offline/unreachable — untouched
)

type FFResult struct {
	Outcome FFOutcome
	OldTip  string // short SHA, set when Outcome == FFUpdated
	NewTip  string
	Detail  string // human-readable extra (error text on fetch-failed)
}

// FastForwardWorktree fetches the upstream of the branch checked out at
// worktreePath and fast-forwards it iff the worktree is clean and the local
// branch is strictly behind. Never rewrites history, never touches a dirty
// tree. (branch-features spec E4)
func FastForwardWorktree(worktreePath string) (FFResult, error)
```
Decision order: upstream lookup (`branchUpstreamRef` on the checked-out branch — `GetCurrentBranch(worktreePath)`) → none = FFNoUpstream; fetch (`fetchRemoteShapedRef`) → warning = FFFetchFailed; dirty check (`HasUncommittedChanges`) = FFDirty; ahead/behind via `git rev-list --left-right --count branch...upstream` → behind-only = ff (`git -C wt merge --ff-only <upstream>`), equal = FFUpToDate, ahead/both = FFDiverged. `error` return only for plumbing failures (not for skip outcomes).

Tests (real repos, E1 fixtures): behind→FFUpdated + file actually appears in worktree + tips set; equal→FFUpToDate; dirty+behind→FFDirty + tree untouched + branch NOT moved; diverged(local commit + remote commit)→FFDiverged untouched; no upstream→FFNoUpstream; broken remote→FFFetchFailed. RED→GREEN; `go test ./internal/git/ -run 'FastForward' -race -count=1 -timeout 300s`; build.
Commit: `feat(git): FastForwardWorktree — gated ff-pull helper (E4)` + Co-Authored-By: Claude.

## Task 2: `UpdateFeature` service + CLI (TDD)

**Files:** `internal/session/feature.go`, `internal/session/feature_test.go`, `cmd/agent-deck/feature_cmd.go` (+help).

```go
type FeatureRepoUpdate struct {
	RepoName string
	Result   git.FFResult
}
// UpdateFeature fast-forwards every repo worktree of an ACTIVE feature.
// Parked features error (no worktrees). Missing worktree dir → Detail notes it,
// Outcome FFFetchFailed is NOT used — add FFMissing? No: report as Detail on
// FFNoUpstream is wrong too — use a dedicated FFOutcome "missing-worktree" in Task 1
// if cheap, else Detail-only on a new outcome. DECISION: add FFMissing FFOutcome
// in this task (one constant + no logic change in FastForwardWorktree itself).
func UpdateFeature(db *statedb.StateDB, name string) ([]FeatureRepoUpdate, error)
```
CLI `feature update <name>`: tabwriter table REPO | OUTCOME | OLD..NEW | DETAIL; exit 0 even with skips (they're outcomes, not errors); exit 1 only on the error return. Help text updated.
Tests: 2-repo feature, one behind one up-to-date → outcomes per repo; parked feature → error. RED→GREEN; `go test ./internal/session/ -run 'UpdateFeature' -race -count=1`; build.
Commit: `feat(feature): UpdateFeature ff-pull across repos + CLI (E4)` + Co-Authored-By: Claude.

## Task 3: TUI hotkey (after E3 fix lands — same home.go)

**Files:** `internal/ui/home.go` (+ test if the package's hotkey tests have precedent).

- Hotkey `U` (shift+u) on a session row belonging to a feature (worktree session): async `UpdateFeature` via the established async-msg pattern (mirror `worktreeFinishResultMsg`): `featureUpdateResultMsg{sessionID, updates, err}`; result → status toast: "feature <name>: N updated, M skipped" + setError on err. Guard: only offer on worktree sessions (feature lookup by session's feature_id or branch); no-op with hint otherwise.
- Run targeted ui tests + build.
- Commit: `feat(ui): U hotkey — ff-pull update for feature sessions (E4)` + Co-Authored-By: Claude.

## Self-review notes
- ff of a checked-out branch mutates worktree files — gates (clean + ff-only) enforced in Task 1 helper, single choke point reused by E5.
- UpdateFeature operates on ACTIVE features only; parked have no worktrees.
- Risk: session running while user presses U — E4 allows it (manual action, clean tree required anyway); E5 adds the not-running gate for background.
