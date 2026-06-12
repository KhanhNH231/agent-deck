# E5: Background Auto-Update — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** Opt-in background ff-pull of feature branches while their sessions are NOT running. Reuses `UpdateFeature` (E4). Triple gate: clean tree (inside FastForwardWorktree) + session not running + ff-only.

**Spec:** E5 of `docs/superpowers/specs/2026-06-12-branch-features-design.md`. **One deliberate deviation:** update results surface via the TUI toast convention + log, NOT the iTerm notify-event pipeline (that infra is per-session status episodes; wiring a new event class through it is disproportionate for v1 — revisit if OS-level notify wanted).
**Branch:** feat/agent-deck-session-ux

## Task 1 (single task): config + gate + timer

**Files:** `internal/session/userconfig.go` (+ its test), `internal/ui/home.go` (+ new `internal/ui/auto_update.go` if cleaner), new `internal/ui/auto_update_test.go`.

### Config (TDD against userconfig test idiom)
`[workspace]` gains: `auto_update = false` (opt-in), `auto_update_interval_minutes = 30` (clamped: <5 → 5). Getters on WorkspaceSettings: `AutoUpdateEnabled() bool`, `AutoUpdateInterval() time.Duration`. Tests: absent → disabled/30m; set → parsed; 2 → clamped 5m.

### Gate decision (pure func, TDD)
```go
// autoUpdateDue reports whether a feature qualifies for a background ff-pull
// tick: auto-update enabled, the owning sessions are all in a non-running
// status (idle/waiting/finished — NEVER running/working), and at least
// interval has elapsed since that feature's last attempt.
func autoUpdateDue(enabled bool, statuses []session.Status, lastAttempt time.Time, interval time.Duration, now time.Time) bool
```
Unit-test the matrix: disabled; any-running; too-soon; due. (Find the real status enum — investigator notes say classify exists; use the actual running/working constant names.)

### Wiring
- Piggyback the existing status/revive tick in home.go (~2519 reviverTick or the status loop — pick the established periodic hook; do NOT add a new ticker unless none fits).
- Per tick: if enabled, group worktree sessions by feature name (branch), collect each feature's session statuses, check `autoUpdateDue` against a `map[string]time.Time` lastAttempt store on Home; for due features set lastAttempt=now and spawn the SAME async cmd as the U hotkey (reuse `featureUpdateResultMsg` + handler — one result path for manual and auto).
- Auto-triggered results: reuse the existing handler; prefix toast with "auto-update" vs manual (add a `auto bool` field to the msg; silent when zero repos changed — only toast when N updated > 0 or err != nil, to avoid 30-minute noise).
- Fetch failures: logged via existing patterns, no toast spam (err≠nil from UpdateFeature still toasts — that's feature-level, rare).

### Verification
RED→GREEN per piece; `go test ./internal/session/ -run 'Workspace|AutoUpdate' -race -count=1`, `go test ./internal/ui/ -run 'AutoUpdate|FeatureUpdate' -race -count=1 -timeout 600s`, `go build ./...`.
Commit: `feat(ui): opt-in background auto-update for feature branches (E5)` + Co-Authored-By: Claude.

## Self-review notes
- Reuses single result path → manual and auto behavior can't drift.
- lastAttempt in-memory only: restart resets the clock — acceptable (worst case an early extra fetch).
- Never-running gate uses live status classification; races (status flips after check) are benign — FastForwardWorktree's clean-tree gate is the hard safety.
