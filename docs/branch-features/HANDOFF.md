# Branch Features — status

**Spec:** `docs/superpowers/specs/2026-06-12-branch-features-design.md`
**Branch:** feat/agent-deck-session-ux

## Shipped

### E1 — Fetch before branch create (4497c788..e41d545e, tests green)

- `fetchRemoteShapedRef(repoDir, ref)` in `internal/git/git.go` (~line 802):
  best-effort fetch when ref = `<configured-remote>/<branch>`; returns warning
  string on failure, "" otherwise. `branchUpstreamRef` companion helper.
- `CreateWorktreeAtStartPoint` → `(createdBranch, warning, err)`; fetches the
  start point. Warning survives the error path (a fetch warning often explains
  the worktree-add failure).
- `ExtendFeature` → `(warning, err)`; CLI prints `warning:` to stderr before
  error handling.
- `CreateWorktree` local arm freshens the branch's upstream tracking ref
  (checkout stays at local tip — ff is E4); remote arm fetches before creating
  the tracking branch; new-branch arm already fresh (#973).
- Coverage verdict (reviewed): start / extend / resume / fork paths all
  freshened or correctly no-op (SHA start points).
- Tests: `internal/git/branch_freshen_test.go` (5),
  `TestExtendFeature_UnreachableRemoteWarnsAndProceeds`,
  `TestExtendFeature_WarningSurvivesWorktreeFailure`. Full `internal/git`
  package green (64s).
- Deliberate deviation from spec: CreateWorktree arms are SILENT best-effort
  (no warning plumbing) — many callers, mirrors #973 precedent. Warning
  surfacing exists only on the extend/AtStartPoint path.

## Remaining sub-projects (spec'd, not started)

- **E2** — per-repo branch, service + CLI (`repo@branch` syntax; ExtendFeature
  gains explicit branch param; CreateMultiRepoWorktrees per-repo branch map).
- **E3** — per-repo branch TUI (newdialog per-repo branch fields).
- **E4** — `feature update` ff-pull (clean+ff-only gates; per-repo outcome
  report; `FastForwardWorktree` helper).
- **E5** — background auto-update (E4 core + triple gate: clean, session not
  running, ff-only; opt-in config).

Build order: E2 → E3, E4 → E5. Each needs its own plan
(per `docs/superpowers/plans/2026-06-12-e1-fetch-before-branch.md` pattern).

## Notes for next session

- One fetch per repo per feature start is accepted for interactive flows;
  E5 must rate-limit (spec notes this).
- E2 will touch `ExtendFeature`'s signature again (branch param) — E1's
  warning return is already in place, keep both.
