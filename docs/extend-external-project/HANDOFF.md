# Handoff: Extend an external project with a new repo + branch (Feature 4)

**Status:** Not started. Investigation done; design not yet brainstormed. Pick up
in a fresh session — start with the `superpowers:brainstorming` skill.
**Branch:** feat/agent-deck-session-ux

## Goal

Take a session/project that was **not created by agent-deck** (an externally
created session, or a plain single-repo session) and **extend it with an
additional repo + branch**, the way the `wsw` tool's `extend` command does —
turning it into (or growing) a multi-repo worktree session.

Reference: `~/Documents/Projects/wsw/` and its
`docs/agent-deck-integration/` (Architecture/PLAN/Workflow). wsw already delegates
session lifecycle to agent-deck; `wsw extend` adds a repo+branch to a feature's
worktree set. This feature brings that capability natively into agent-deck's TUI.

## How agent-deck models multi-repo today

- `Instance.MultiRepoEnabled`, `Instance.AdditionalPaths`, `Instance.ProjectPath`,
  `Instance.MultiRepoTempDir`, `Instance.MultiRepoWorktrees` (`internal/session/instance.go`).
- Creation: `home.go` ~9188 — two branches:
  - **worktree** (`worktreeBranch != ""`): `session.CreateMultiRepoWorktrees`
    builds a real worktree per repo under
    `~/.agent-deck/multi-repo-worktrees/<branch>-<id>/`. **Hardened in Feature 1**
    (commit `dfe6eb1e`): a git repo that can't be isolated is now fatal, never
    silently symlinked.
  - **no-worktree**: symlinks live repos into the temp dir.
- Edit existing paths: `p` → `EditPathsDialog`
  (`internal/ui/editpaths_dialog.go`), applied by
  `home.go:8589 applyMultiRepoPathChanges`.

## ⚠️ Landmine — the edit-paths apply path symlinks live repos

`applyMultiRepoPathChanges` (home.go:8589) **always** does
`os.Symlink(liveRepoPath, linkPath)` for every path — it does **not** create
worktrees, even when the session IS a worktree session. This is the same
live-repo-symlink hazard Feature 1 just removed from `CreateMultiRepoWorktrees`,
still present here.

**Implication for extend:** if "extend" reuses or resembles `applyMultiRepoPathChanges`,
adding a repo to a worktree session would symlink the live repo (edits leak into
main repo). Extend MUST, for a worktree session, create a real worktree for the
new repo (reuse `git.CreateWorktreeWithSetup` / `session.CreateMultiRepoWorktrees`
semantics) and apply the same fail-loud-on-failure rule. Consider fixing
`applyMultiRepoPathChanges` for worktree sessions as part of this work (or first,
as a prerequisite bug-fix commit).

## Design questions to brainstorm (with the user)

1. **Entry point:** new hotkey / a CLI subcommand (`agent-deck extend <id>
   <repo> [--branch X]`) / extend the existing Edit-Paths dialog with an "add
   repo + branch" affordance? (wsw parity suggests a CLI subcommand + TUI action.)
2. **Branch semantics:** new branch name (create worktree on new branch) vs attach
   to an existing branch. Reuse the new-session worktree-branch flow.
3. **Promote single → multi:** if the target session is single-repo (not yet
   multi), extending promotes it: create `MultiRepoTempDir`, move the existing
   repo's worktree in, set `MultiRepoEnabled`. Define this migration carefully.
4. **Externally-created sessions:** what state do they have? (May lack
   `WorktreeRepoRoot`, branch, etc.) Determine the minimum fields needed and how
   to backfill. This is the crux of "not created in agent-deck."
5. **Restart vs hot-add:** `applyMultiRepoPathChanges` restarts the session
   (`sessionRestartedMsg`). Is a restart acceptable on extend, or hot-add the dir?
6. **Conductor / CLAUDE.md:** new-session path calls
   `session.ApplyMultiRepoClaudeContext` to emit a parent CLAUDE.md describing the
   layout (home.go:9260). Extend must regenerate it for the new repo set.
7. **Teardown:** removing an extended repo → `git.RemoveWorktree` + unregister, like
   Feature 1's rollback.

## Files likely touched

- `internal/session/multi_repo_worktree.go` (reuse / extend creation+rollback)
- `internal/session/instance.go` (promote single→multi; field backfill)
- `internal/ui/home.go` (entry point, apply, restart, CLAUDE.md regen)
- `internal/ui/editpaths_dialog.go` (if extending that dialog)
- `cmd/agent-deck/*` (if adding a CLI subcommand)
- Fix `applyMultiRepoPathChanges` worktree handling (prerequisite)
- >5 files → **split** per CLAUDE.md: (a) prerequisite edit-paths worktree fix,
  (b) extend core (promote + add-repo-worktree), (c) entry-point UX (TUI/CLI).

## Verification plan

- Unit: promote single→multi produces a worktree (not symlink) for the new repo;
  fail-loud on un-isolable git repo (mirror Feature 1 tests); CLAUDE.md regen
  lists the new repo.
- Manual: extend a real external session with a second repo+branch; confirm the
  added repo is a real worktree (`git -C <repo> worktree list` shows it; the
  session dir entry is a directory, not a symlink to the live repo). Label "not
  yet verified" until run.

## Related

- Feature 1 spec: `docs/superpowers/specs/2026-06-10-multirepo-worktree-no-symlink-design.md`
- wsw integration: `~/Documents/Projects/wsw/docs/agent-deck-integration/`
