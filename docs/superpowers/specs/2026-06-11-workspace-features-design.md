# Workspace Features — wsw absorbed into agent-deck

**Date:** 2026-06-11
**Status:** Approved design (umbrella spec; each sub-project gets its own plan)
**Branch:** feat/agent-deck-session-ux

## Goal

Absorb the `wsw` workspace manager (`~/Documents/Projects/wsw`) into agent-deck
natively, then retire wsw. All worktree sessions become "features" managed under
a configurable workspace root (default `~/Documents/Projects/workspaces`).

## Locked decisions

| Decision | Choice |
|---|---|
| wsw fate | Replaced entirely; re-implemented in Go inside agent-deck |
| Managed root scope | ALL worktree sessions (not just multi-repo) |
| Repo manifest | TOML in `~/.agent-deck/config.toml` (`[[repos]]`) |
| Feature modeling | First-class entity in state.db (schema v11) |
| UX priority | TUI-first; CLI subcommands follow |
| Migration | Fresh start — no wsw importer; old features finish life in wsw |
| Uniformity | Every worktree session gets a feature entity (single-repo = 1-repo feature, auto-named from branch) |
| Slack | Out of scope — covered by Feature 3 (`docs/slack-notifications/HANDOFF.md`) |
| rebase / status | Out of scope — port later if missed |

## Decomposition

Each sub-project: own spec refinement → plan → implementation → verification.
Order matters (B needs A; C and D need B).

| # | Sub-project | Delivers |
|---|---|---|
| A | Workspace root + repo manifest | `[workspace].root` config; `[[repos]]` manifest; worktree placement rerouted under root |
| B | Feature entity + lifecycle | `features`/`feature_repos` tables; start / stop(park) / resume; TUI dialog + rows |
| C | Extend | Add repo+branch to any feature; prerequisite fix for `applyMultiRepoPathChanges` symlink landmine (`internal/ui/home.go:8589`); subsumes `docs/extend-external-project/HANDOFF.md` |
| D | Docs scaffolding + conductor wiring | Feature CLAUDE.md, handoff.md, contracts/, worker briefs; conductor session per feature |

## Disk layout

```
~/Documents/Projects/workspaces/            # configurable root
└── <feature>/
    ├── worktrees/
    │   ├── <repo-a>/                       # real git worktree, never a symlink
    │   └── <repo-b>/
    ├── CLAUDE.md                           # feature context (D)
    ├── docs/handoff.md                     # survives stop
    ├── contracts/                          # conductor mode only (D)
    └── workers/<repo>.md                   # conductor mode only (D)
```

- Single-repo session: same shape, one entry under `worktrees/`.
- `stop` deletes `worktrees/*` only; docs and feature dir survive.
- Feature dir name = feature name (default: branch name slug).

## Configuration

```toml
# ~/.agent-deck/config.toml
[workspace]
root = "~/Documents/Projects/workspaces"    # default when key absent

[[repos]]
name = "crawler-v3-golang"
path = "~/Documents/Projects/crawler-v3-golang"
default_base = "origin/master"
```

- `[worktree].default_location` (sibling/subdirectory) honored only when
  `[workspace]` section absent — legacy fallback; document deprecation.
- Bare-repo resolution, `.worktreeinclude`, `.agent-deck/worktree-setup.sh`
  unchanged — only placement path changes.

## Data model (schema v11)

```
features:        id, name (unique), state (active|parked), root_path,
                 conductor (bool), created_at, updated_at
feature_repos:   feature_id, repo_name, repo_path, branch, base_ref, worktree_path
instances:       + feature_id (nullable; non-worktree sessions stay NULL)
```

- Feature rows survive session deletion → park/resume is clean.
- Manifest repo data is **snapshotted** into `feature_repos` at start time;
  later manifest edits never break parked features.

## Lifecycle semantics

### start
Dialog inputs: feature name, repo picker (from manifest), branch (default =
feature name slug), base ref (default from manifest per repo), conductor toggle.
Steps: feature row → feature dir → worktree per repo (reuse
`session.CreateMultiRepoWorktrees` semantics: fail-loud on un-isolable repo,
rollback created worktrees in reverse on partial failure) → docs scaffold (D) →
session(s). Default: one session with all worktrees via `--add-dir`. Conductor
mode: one worker session per repo + conductor session.

### stop (park)
Kill sessions → `git worktree remove` each (refuse if dirty unless forced;
branches survive in their repos) → keep docs/CLAUDE.md → `state = parked`.
Safety (ported from wsw): refuse if CWD inside a target worktree; refuse if
feature root is a symlink.

### resume
Recreate worktrees from existing branches → recreate sessions (user choice:
fresh vs restore conversation — reuse existing restore-sessions feature) →
`state = active`.

### extend
Pick repo from manifest + branch → create real worktree (never symlink) →
regenerate feature CLAUDE.md → restart session (hot-add deferred).
**Prerequisite:** fix `applyMultiRepoPathChanges` (home.go:8589) which today
always symlinks live repos even for worktree sessions.

### delete
Park + remove docs dir + delete feature rows. Branch deletion stays manual.

## TUI

- New-session worktree flow auto-creates a feature named from the branch —
  no extra ceremony for quick worktrees.
- Explicit "new feature" mode: multi-select repo picker + conductor toggle.
- Feature = TUI group (GroupPath = feature name). Parked features render in a
  collapsed section with a state badge.
- Hotkeys on feature rows: park, resume, extend.
- CLI follows: `agent-deck feature start|stop|resume|extend|list`.

## Docs scaffolding + conductor (D)

- Embedded Go templates render `CLAUDE.md` (repo list, branches, layout —
  extends `session.ApplyMultiRepoClaudeContext`) and `docs/handoff.md` stub.
- Conductor mode adds `contracts/` and `workers/<repo>.md` briefs.
- Conductor reuses existing agent-deck conductor infrastructure, bound to the
  feature via `feature_id`.

## Safeguards

- No-symlink fail-loud rule (Feature 1, commit `dfe6eb1e`) applies to every
  path that materializes a repo into a feature dir.
- Partial-start rollback removes created worktrees in reverse order.
- Dirty-worktree check before park; `--force` to override.
- CWD-inside-worktree and feature-root-symlink refusals on park/delete.
- Existing S1–S5 data-loss protections untouched.

## Testing

- A: placement resolution + manifest parse/validate (unit).
- B: feature CRUD; park→resume round-trip against real temp git repos (unit,
  behavior-level through public API).
- C: extend on single-repo promotes to multi with a real worktree; no-symlink
  regression test mirroring Feature 1's.
- D: template render snapshot per mode.
- Manual checklist per sub-project; claims labeled "not yet verified" until run.

## Out of scope

- wsw `rebase` / `status` commands (port later on demand).
- Slack notify (Feature 3 owns it).
- wsw workspace importer.
- Hot-add of dirs to a live session on extend (restart is acceptable v1).

## Related

- `docs/extend-external-project/HANDOFF.md` (Feature 4 — subsumed by C)
- `docs/slack-notifications/HANDOFF.md` (Feature 3)
- `docs/superpowers/specs/2026-06-10-multirepo-worktree-no-symlink-design.md` (Feature 1)
- `~/Documents/Projects/wsw/` (reference implementation, to be retired)
