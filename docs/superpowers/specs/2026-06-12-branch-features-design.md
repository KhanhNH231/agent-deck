# Branch Features — fresh refs, per-repo branches, ff-pull updates

**Date:** 2026-06-12
**Status:** Approved design (umbrella spec; each sub-project gets its own plan)
**Branch:** feat/agent-deck-session-ux

## Goal

Three workflow gaps around feature branches:

1. Worktrees are created from **stale base refs** (`CreateWorktreeAtStartPoint`
   uses local remote-tracking refs as-is — `git.go:446-466`).
2. Multi-repo features force a **single shared branch** across all repos
   (`multi_repo_worktree.go:24`, `feature.go:171-184`); real work needs e.g.
   `wms-service@feat/multi-packages-pre-gwp` + `analytics@feat/multi-packages`.
3. **No way to update** a feature's branches once created — no CLI, no hotkey,
   no background refresh.

## Locked decisions

| Decision | Choice |
|---|---|
| Update semantics | **ff-pull from `origin/<same-branch>`** (teammates' commits). Never rebase; `feature rebase` out of scope. |
| Update triggers | All three: fetch-before-create (E1), manual `feature update` + hotkey (E4), background auto-update (E5, opt-in) |
| Per-repo branch surfaces | Both CLI and TUI (E2 service+CLI first, E3 TUI after) |
| Offline behavior at create | Warn loudly + proceed with stale ref (never block offline work) |
| Schema | No change — `feature_repos.branch` is already per-row; only creation paths populate it uniformly today |

## Safety invariants (apply to every sub-project)

- **ff-only, never rewrite.** No rebase, no `--force`, no reset.
- **Never touch a dirty worktree.** Dirty → skip + report.
- **Never ff under an actively-running session** (E5); manual E4 requires clean
  worktree regardless of session state (ff rewrites checked-out files).
- **Diverged (local ahead or both moved) → always skip**, manual resolution.
- Existing S1–S5 data-loss protections and the no-symlink fail-loud rule untouched.

## Decomposition (build order: E1 → E2 → E3, E4 → E5)

### E1 — Fetch before branch create

Every path that materializes a worktree from a base ref fetches that ref's
remote first: feature start (TUI + CLI), extend, resume.

- `git.go`: before `CreateWorktreeAtStartPoint` resolves `baseRef`, run
  `git fetch --quiet <remote> <branch>` when `baseRef` parses as
  `<remote>/<branch>` against a configured remote. Non-remote refs (local
  branch, SHA, tag) → no fetch.
- Existing-branch worktree path (`CreateWorktree` branch-exists arm): fetch the
  branch's upstream if it has one (picks up teammates' commits before checkout).
- Fetch failure (offline, auth, missing remote branch): **warn + proceed**.
  Warning carries repo name + ref + error; surfaced in TUI flows and CLI stderr.
- `freshOriginDefaultBranchRef` (git.go:779-793) already fetches for the
  new-orphan-branch path — unify so fetch logic lives in one helper.

### E2 — Per-repo branch: service + CLI

- Syntax: `repo[@branch]` everywhere a repo is named:
  `agent-deck feature start <name> wms-service@feat/multi-packages-pre-gwp analytics@feat/multi-packages data-load`
  (bare repo → branch = feature-name slug, current behavior).
- `ExtendFeature(db, name, repoName, repoPath, baseRef, branch string)` —
  branch becomes an **explicit required** parameter (no optional omission);
  callers pass the slug default deliberately. CLI: `feature extend <name> <repo>[@branch] [--base ref]`.
- `CreateMultiRepoWorktrees`: single `branch string` → per-repo
  `branches map[string]string` (keyed by repo path; absent key is a
  programming error → fail loud, no silent default).
- `feature_repos.branch` written per repo from the map. Park/resume already
  read per-row branch — resume recreates mixed-branch features correctly
  (verify with test; suspected to already work).

### E3 — Per-repo branch: TUI

- New-feature dialog (`newdialog.go`): repo picker rows gain an editable branch
  field, prefilled with the feature slug. Single-branch fast path unchanged
  (edit nothing → identical to today).
- Display: feature/session rows show per-repo branches where they differ
  (`repo@branch` list in detail pane).

### E4 — `feature update` (manual ff-pull)

- CLI: `agent-deck feature update <name>`; TUI: hotkey on feature row.
- Per repo in the feature: `git fetch origin <branch>` →
  if worktree clean AND `branch..origin/branch` is fast-forwardable →
  `git -C <worktree> merge --ff-only origin/<branch>`.
- Per-repo outcome report: `updated <old>..<new> | up-to-date | dirty-skip |
  diverged-skip | fetch-failed`. CLI prints table; TUI shows summary toast +
  detail in preview.
- New `git.go` helper `FastForwardWorktree(worktree, branch) (Outcome, error)` —
  single deep function, reused by E5.

### E5 — Background auto-update

- E4 core on a timer inside the existing status loop (`home.go`).
- Triple gate per repo: worktree clean + owning session **not running**
  (reuse status classification: idle/waiting OK, running → skip) + ff-only.
- Config (`userconfig.go`): `[workspace] auto_update = false` (opt-in),
  `auto_update_interval_minutes = 30` (min 5).
- On applied update: notification via existing notify infra (event text
  `repo branch a1b2c3..d4e5f6`); per-episode dedupe like other notify events.
- Failure modes (fetch error) are silent at tick level, logged; no notify spam.

## Testing (per sub-project, behavior-level, real temp git repos with remotes)

- E1: repo with stale remote-tracking ref; create worktree; assert new branch
  roots at the REMOTE's current tip, not the stale local ref. Offline case:
  fetch fails → worktree still created + warning returned.
- E2: feature start with two repos on different branches → `feature_repos`
  rows + actual checked-out branches differ correctly; park → resume
  round-trip preserves mixed branches; extend with `repo@branch`.
- E3: dialog state test (per-repo branch edit) + snapshot of rendered rows.
- E4: matrix — clean+behind→updated; clean+diverged→skip; dirty→skip;
  no-upstream→report; offline→fetch-failed. Assert worktree file state actually
  changes only in the `updated` case.
- E5: unit-test the gate decision function (status × dirty × config) — the
  timer wiring itself gets a focused integration test with a fake clock if the
  status loop supports injection, else manual checklist.

## Out of scope

- `feature rebase` (rebase onto base) — revisit on demand.
- Auto-update for non-feature (plain) sessions.
- Conflict resolution UX — diverged always means manual.

## Related

- `docs/superpowers/specs/2026-06-11-workspace-features-design.md` (feature entity, lifecycle)
- Investigation evidence: `newdialog.go:1020`, `multi_repo_worktree.go:24`,
  `feature.go:171-184`, `git.go:378-426,446-466,779-793`, `feature_cmd.go:99`
