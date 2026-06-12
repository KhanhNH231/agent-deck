# E3: TUI Per-Repo Branch — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** New-session worktree dialog supports per-repo branch overrides for multi-repo sessions. Main branch input stays = default for all repos + feature name; per-repo override changes only that repo.

**Spec:** `docs/superpowers/specs/2026-06-12-branch-features-design.md` (E3). E2 landed: `session.MultiRepoBranches`, `UniformBranches`, `CreateMultiRepoWorktrees(allPaths, parentDir, branches, timeout)`.

**Branch:** feat/agent-deck-session-ux

## Task 1: Dialog model + submit wiring (TDD where the package has test precedent)

**Files:** `internal/ui/newdialog.go` (branch input ~line 1020, multi-repo path list model), `internal/ui/home.go` (~9274 submit site), matching `internal/ui/*_test.go` (find dialog state-test precedent — e.g. existing newdialog tests; mirror their style).

Implementation direction (adapt to actual model after reading the file):
- Dialog state: per-additional-path branch override map `map[string]string` (empty = use main branch input). UX: on the paths/repo list rows, a key (suggest `b`) opens an inline branch input for the focused repo row, prefilled with the main branch value; ESC cancels; non-git paths reject the override with a status hint.
- Render: repo rows with an override show `@<branch>` suffix (dim style consistent with the file's palette).
- Submit: build `session.MultiRepoBranches` = main branch for every path, then apply overrides; pass to `CreateMultiRepoWorktrees` instead of `UniformBranches(...)` at home.go:9274. Single-repo + no-override flow must produce byte-identical behavior to today.
- Feature registration: unchanged — feature name remains the MAIN branch input; per-repo branches flow via `MultiRepoWorktree.Branch` rows (E2 made this per-repo).
- Tests: dialog-state unit tests (override set/clear/reject-non-git; submit map correctness — main-only, mixed, all-overridden). Follow the package's existing dialog test idiom; no TTY needed for state tests.
- Run: `go test ./internal/ui/ -run '<new test names + nearest dialog suite>' -race -count=1 -timeout 300s`; `go build ./...`.
- Commit: `feat(ui): per-repo branch overrides in worktree dialog (E3)` + Co-Authored-By: Claude.

## Out of scope
Parked-feature TUI surface (separate backlog), edit-paths per-repo branch (reconcile keeps session branch for kept worktrees; added repos via dialog get the session's branch — extend CLI covers custom).

## Self-review notes
- The only behavior change is additive (override map); default path identical.
- Risk: newdialog.go is large — implementer must follow existing focus/input patterns, not invent new widget plumbing.
