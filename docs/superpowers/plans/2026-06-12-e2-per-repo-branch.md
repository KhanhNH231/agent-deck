# E2: Per-Repo Branch (service + CLI) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or superpowers:executing-plans. Checkbox steps.

**Goal:** A feature can hold repos on different branches: `wms-service@feat/multi-packages-pre-gwp` + `analytics@feat/multi-packages`. Service + extend-CLI surface; TUI is E3.

**Architecture:** `CreateMultiRepoWorktrees`/`ReconcileMultiRepoWorktrees` take a per-path branch map (fail-loud on missing key; `UniformBranches` constructor preserves single-branch call sites). `ExtendFeature` gains an explicit `branch` parameter. CLI `feature extend` accepts `repo[@branch]`. Mixed-branch park→resume round-trip pinned by test (ResumeFeature already reads `feature_repos.branch` per row — verify, fix if not).

**Spec:** `docs/superpowers/specs/2026-06-12-branch-features-design.md` (E2)
**Branch:** feat/agent-deck-session-ux

---

## Task 1: Branch map through Create/Reconcile MultiRepoWorktrees (TDD)

**Files:** `internal/session/multi_repo_worktree.go`, its test file(s) (find via grep `CreateMultiRepoWorktrees` in `internal/session/*_test.go`), `internal/ui/home.go` (two call sites: Create ~9274, Reconcile — grep it).

- New type + constructor in `multi_repo_worktree.go`:
```go
// MultiRepoBranches maps each original repo path to the branch its worktree
// should be on. Every git-repo path MUST have an entry — a missing key is a
// programming error and fails the whole operation (no silent default).
type MultiRepoBranches map[string]string

// UniformBranches builds a MultiRepoBranches putting every path on the same
// branch — the single-branch fast path used by the TUI default flow.
func UniformBranches(paths []string, branch string) MultiRepoBranches { ... }
```
- `CreateMultiRepoWorktrees(allPaths, parentDir string, branches MultiRepoBranches, setupTimeout)` — per git-repo path: `branch, ok := branches[p]; if !ok || branch == "" { result.Err = fmt.Errorf("no branch specified for repo %s", p); rollback; return }`. Non-git symlink arm unchanged. `MultiRepoWorktree.Branch` gets the per-repo value.
- `ReconcileMultiRepoWorktrees(parentDir, branches MultiRepoBranches, existing, newPaths, setupTimeout)` — same lookup for ADDED git repos only (kept worktrees keep their branch).
- Call sites in home.go: wrap current single `worktreeBranch` with `session.UniformBranches(...)` — behavior identical.
- TDD: failing test first — two real repos, branch map with different branches → assert each worktree checked out on ITS branch (`git -C wt rev-parse --abbrev-ref HEAD`); missing-key case → Err + rollback (no worktrees left). RED → implement → GREEN. Mirror existing fixtures in the package tests.
- Run: `go test ./internal/session/ -run 'MultiRepo' -race -count=1 -timeout 300s`; `go build ./...`.
- Commit: `feat(workspace): per-repo branch map in multi-repo worktree create/reconcile (E2)` + Co-Authored-By: Claude.

## Task 2: ExtendFeature branch param + CLI repo@branch (TDD)

**Files:** `internal/session/feature.go` (ExtendFeature ~171), `internal/session/feature_test.go`, `cmd/agent-deck/feature_cmd.go` (extend handler + help text).

- `ExtendFeature(db, name, repoName, repoPath, baseRef, branch string) (warning string, err error)` — branch REQUIRED: empty → keep today's fallback (feature's branch / name) but make it explicit at the CALL SITE, i.e. CLI computes the default and always passes a non-empty branch; service errors on empty branch (`errors.New("branch must not be empty")`). Replaces the silent `branch := name; if repos[0].Branch != ""...` logic — move that derivation to an exported helper `DefaultFeatureBranch(repos []statedb.FeatureRepoRow, name string) string` used by the CLI.
- CLI: `feature extend <name> <repo>[@branch] [--base ref]`. Parse helper in feature_cmd.go:
```go
// splitRepoBranch splits "repo@branch" — repo may be a path; branch may
// contain "/" (feat/x). Split on the FIRST "@" only. No "@" → branch "".
func splitRepoBranch(arg string) (repo, branch string)
```
(`@` in paths is unusual; document the limitation in help text.) When branch part empty → `session.DefaultFeatureBranch(...)`.
- Update help text + the feature.go:198 call site comment.
- TDD: service test — extend with explicit branch ≠ feature name → `feature_repos` row has that branch AND worktree is on it; empty-branch service call errors. RED → GREEN. Existing extend tests updated to pass the derived default explicitly.
- Run: `go test ./internal/session/ -run 'ExtendFeature' -race -count=1`; `go build ./...`.
- Commit: `feat(feature): explicit per-repo branch on extend, repo@branch CLI syntax (E2)` + Co-Authored-By.

## Task 3: Mixed-branch park→resume round-trip (test-first; fix only if red)

**Files:** `internal/session/feature_test.go`; possibly `internal/session/feature.go` (ResumeFeature) if the test exposes a bug.

- Test: register feature with two real repos on DIFFERENT branches (reuse Task 1/2 fixtures) → `ParkFeature` → assert worktrees gone, branches survive in repos → `ResumeFeature` → assert each worktree recreated on its ORIGINAL per-repo branch.
- Expected: passes already (ResumeFeature reads per-row branch). If RED: fix ResumeFeature to use `r.Branch` per row, never a feature-level branch.
- Run: `go test ./internal/session/ -run 'Feature' -race -count=1 -timeout 300s`; `go build ./...`.
- Commit (test-only or test+fix): `test(feature): pin mixed-branch park/resume round-trip (E2)` + Co-Authored-By.

---

## Self-review notes
- Spec coverage: branch map (T1), explicit extend branch + syntax (T2), resume correctness (T3). RegisterWorktreeSessionFeature needs NO change — FeatureRepo rows are built from per-worktree `MultiRepoWorktree.Branch`, which T1 makes per-repo; feature NAME stays = the main branch input (TUI E3 decision).
- Optional-param rule honored: branch is explicit/required at the service boundary; defaults computed visibly at call sites.
- Risk: `@` in repo paths — split on first `@`, documented; manifest names are the common case.
