# Workspace Features (wsw absorption) — status

**Spec:** `docs/superpowers/specs/2026-06-11-workspace-features-design.md`
**Plan:** `docs/superpowers/plans/2026-06-11-workspace-features.md`
**Branch:** feat/agent-deck-session-ux (commits 846b116a..25c349b2)

## Shipped (all tested, focused suites green)

- **A — config:** `[workspace].root` + `[[repos]]` manifest in config.toml
  (`internal/session/userconfig.go`); `git.FeatureWorktreePath/FeatureDir`;
  single-repo placement via `resolveWorktreeTarget`, multi-repo parentDir =
  `<feature>/worktrees` (cwd is worktrees dir so MultiRepoTempDir cleanup can
  never delete docs).
- **B — entity/lifecycle:** schema v11 (`features`, `feature_repos`,
  `instances.feature_id`); `internal/session/feature.go`
  (Register/Park/Resume/Delete, all-or-nothing park gates); auto-register on
  worktree session creation; `agent-deck feature
  list|park|resume|extend|conductor|delete`; worktree-finish keeps feature
  table consistent (keepBranch→parked, branch deleted→row dropped).
- **C — extend + landmine fix:** `ReconcileMultiRepoWorktrees` replaces the
  destructive symlink logic in `applyMultiRepoPathChanges` (was: RemoveAll on
  real worktrees + live-repo symlinks). Edit-paths (`p`) is the TUI extend.
  `ExtendFeature` + CLI for manifest-driven extend.
- **D — scaffolding/conductor:** `ScaffoldFeatureDocs` (CLAUDE.md, handoff.md,
  contracts/, workers briefs; never overwrites); `feature conductor <name>`
  reuses `conductor setup` with feature CLAUDE.md as identity.

## Deferred (follow-ups)

1. **TUI parked-feature surface:** parked features have no TUI list/badge;
   resume is CLI-only. Candidate: collapsed "Parked" section + `R` hotkey.
2. **Hot-add on extend:** session restart required after extend (spec-accepted v1).
3. **Conductor worker sessions:** `feature conductor` scaffolds briefs +
   conductor; per-repo worker session spawning is manual.
4. **wsw retirement:** old wsw still works for its existing features (fresh
   start, no importer). Retire once active wsw features finish.
5. **wsw rebase/status:** not ported (out of scope per spec).
6. **Stale worktree registrations on raw session delete:** deleting a
   multi-repo feature session via plain session-delete RemoveAll's
   `<feature>/worktrees` without `git worktree remove`, so a later
   `feature resume` can fail until `git -C <repo> worktree prune`.
   Fix candidate: make ResumeFeature prune before recreate, or route
   session-delete cleanup through git.RemoveWorktree for feature sessions.

## Known pre-existing test failures (NOT from this work)

`TestUpdateStatus_CLIvsTUIParity_*`, `TestIssue1147_*`, `TestIssue1187_*` fail
identically on base commit 77ce903d — environmental (tmux/sandbox).

## Manual verification still pending

Real-TUI run with `[workspace]` configured: create worktree session → dirs
under root; finish-with-keep-branch → parked; `feature resume` → worktree back.
Service-level equivalents are covered by tests in
`internal/session/feature_test.go` and `multi_repo_reconcile_test.go`.
