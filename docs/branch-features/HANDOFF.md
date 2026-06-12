# Branch Features — status

**Spec:** `docs/superpowers/specs/2026-06-12-branch-features-design.md`
**Branch:** feat/agent-deck-session-ux
**Status: ALL FIVE SUB-PROJECTS SHIPPED (E1–E5), 2026-06-12.** Test suites green
(focused, `-race`); full-package sweep results in final session summary.

## Shipped

### E1 — Fetch before branch create (4497c788, b00ad2b0, 3c688867, e41d545e)
- `fetchRemoteShapedRef` + `branchUpstreamRef` in `internal/git/git.go`; every
  worktree-creation arm (start point / remote-tracking / existing-local /
  new-orphan) freshens its ref; offline → warn + proceed.
- `CreateWorktreeAtStartPoint` → `(created, warning, err)`; `ExtendFeature` →
  `(warning, err)`; extend CLI prints warning to stderr BEFORE error (a failed
  fetch often explains the failure).
- Deviation: `CreateWorktree` arms are silent best-effort (#973 precedent).

### E2 — Per-repo branch, service + CLI (30c9f0d5, 6d6eadea, 89112c2b, 6d0ec3b0)
- `MultiRepoBranches` map + `UniformBranches`; Create/Reconcile take per-path
  branches; missing key = fail-loud + rollback; Reconcile validates keys BEFORE
  any removal (review-caught ordering bug).
- `ExtendFeature(..., branch)` explicit-required; `DefaultFeatureBranch`
  computes the visible default at call sites; CLI `feature extend <name>
  <repo>[@branch]` (first-@ split, documented limitation).
- Mixed-branch park→resume round-trip pinned (ResumeFeature was already
  per-row correct).

### E3 — TUI per-repo branch (c96f7607, 76459835)
- New-session dialog, multi-repo list: `b` on a git row → inline branch
  override (prefilled with main branch; save-equal clears); `@branch` suffix
  render; submit builds the branches map (zero overrides ≡ UniformBranches,
  pinned).
- Review-caught: stale overrides on path delete/edit; **mid-string `~/`
  normalization divergence** — override keying now shares `normalizeRepoPath`
  with `GetMultiRepoPaths` (single normalizer; silent-fallback ban enforced).

### E4 — `feature update` ff-pull (9b680b5d, a7fd4e5c, 66d261e8)
- `git.FastForwardWorktree` — single gated choke point: upstream→fetch→dirty→
  ahead/behind→`merge --ff-only`; outcomes updated/up-to-date/dirty-skip/
  diverged-skip/no-upstream/fetch-failed/missing-worktree.
- `session.UpdateFeature` — per-repo outcomes, never aborts mid-feature;
  parked → error. CLI `feature update <name>` outcome table.
- TUI hotkey `U` on worktree sessions (hotkeys.go + help.go + preview hint);
  async via `featureUpdateResultMsg`.

### E5 — Background auto-update (9ec92c19, fd6bb3da)
- Opt-in `[workspace] auto_update = true`, `auto_update_interval_minutes = 30`
  (clamp ≥5). Piggybacks reviverTick (60s); per-feature lastAttempt cooldown.
- Gate `autoUpdateDue`: blocks Running/Starting only (Error/Stopped/Queued/
  Waiting/Idle allowed — documented rationale in auto_update.go); hard safety
  stays FastForwardWorktree's clean-tree gate.
- Auto results reuse the manual result path (`auto` flag): silent unless
  updated>0 or err. Deviation from spec: toast+log, not iTerm notify pipeline.
- **`fetchTimeout = 30s`** on ALL remote fetches (CommandContext) — hung remote
  can no longer leak goroutines; hang path unit-tested via `ext::sleep 60`
  transport (returns in ~1.2s with 1s test timeout).

## Known limitations / follow-ups

1. FFDirty/FFDiverged `Detail` is empty — table could show ahead/behind counts
   (advisory from review; cheap UX add).
2. `@` in repo paths: first-@ split limitation (help text documents).
3. lastAttempt map is in-memory — restart = early extra fetch (accepted).
4. Manual TUI verification pending: E3 override UX + E5 background behavior in
   a real terminal with `[workspace]` + `auto_update` configured. Service
   layers are test-covered; the dialog interaction itself is state-tested only.
5. Parked-feature TUI surface still on the older workspace-features backlog
   (`docs/workspace-features/HANDOFF.md` #1) — untouched by this work.
