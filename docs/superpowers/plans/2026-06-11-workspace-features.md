# Workspace Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Absorb wsw into agent-deck: configurable workspace root, repo manifest, first-class feature entity with park/resume lifecycle, extend, docs scaffolding.

**Architecture:** Four sub-projects (A→D) per spec `docs/superpowers/specs/2026-06-11-workspace-features-design.md`. A reroutes worktree placement under `[workspace].root` and adds a `[[repos]]` manifest. B adds `features`/`feature_repos` tables (schema v11), a feature service (create/park/resume/delete), auto-registration on worktree session creation, TUI park/resume + CLI. C fixes the `applyMultiRepoPathChanges` symlink landmine and adds extend. D scaffolds feature docs.

**Tech Stack:** Go, SQLite (modernc), BurntSushi/toml, bubbletea TUI, real-git-repo tests via `t.TempDir()`.

---

## Sub-project A — Workspace root + repo manifest

### Task A1: WorkspaceSettings + repo manifest in userconfig

**Files:**
- Modify: `internal/session/userconfig.go` (UserConfig struct ~line 132; new types near WorktreeSettings ~1338; accessor near GetWorktreeSettings ~2805)
- Test: `internal/session/userconfig_workspace_test.go` (create)

- [ ] **Step 1: Failing test**

```go
package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceSettingsFromTOML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := filepath.Join(home, ".agent-deck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := `
[workspace]
root = "~/ws"

[[repos]]
name = "alpha"
path = "~/repos/alpha"
default_base = "origin/main"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	InvalidateUserConfigCache()
	ws := GetWorkspaceSettings()
	if !ws.Enabled() {
		t.Fatal("workspace should be enabled when root set")
	}
	if got, want := ws.RootDir(), filepath.Join(home, "ws"); got != want {
		t.Fatalf("RootDir = %q, want %q", got, want)
	}
	repo, ok := FindManifestRepo("alpha")
	if !ok || repo.PathExpanded() != filepath.Join(home, "repos", "alpha") || repo.DefaultBase != "origin/main" {
		t.Fatalf("manifest repo = %+v ok=%v", repo, ok)
	}
}

func TestWorkspaceSettingsAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	InvalidateUserConfigCache()
	if GetWorkspaceSettings().Enabled() {
		t.Fatal("workspace must be disabled when section absent")
	}
}
```

(Adjust cache-invalidation helper name to whatever userconfig.go exposes; if none, add one.)

- [ ] **Step 2: Run, expect FAIL (undefined symbols)**

`go test ./internal/session/ -run TestWorkspaceSettings -v`

- [ ] **Step 3: Implement**

```go
// In UserConfig struct:
	Workspace WorkspaceSettings `toml:"workspace"`
	Repos     []RepoDef         `toml:"repos"`

// WorkspaceSettings configures the managed workspace root (wsw absorption).
// When Root is non-empty, ALL worktree sessions are placed under
// <root>/<feature>/worktrees/<repo-name>/ and [worktree].default_location
// is ignored (legacy fallback only when this section is absent).
type WorkspaceSettings struct {
	Root string `toml:"root"`
}

func (w WorkspaceSettings) Enabled() bool { return strings.TrimSpace(w.Root) != "" }

func (w WorkspaceSettings) RootDir() string { return expandHome(w.Root) }

// RepoDef is one entry of the [[repos]] manifest: a named repo that feature
// dialogs/commands can pick without typing paths.
type RepoDef struct {
	Name        string `toml:"name"`
	Path        string `toml:"path"`
	DefaultBase string `toml:"default_base"`
}

func (r RepoDef) PathExpanded() string { return expandHome(r.Path) }

func GetWorkspaceSettings() WorkspaceSettings
func GetManifestRepos() []RepoDef
func FindManifestRepo(name string) (RepoDef, bool)
```

`expandHome`: reuse existing helper if present (grep `strings.HasPrefix(.*"~/"`); else add one. Accessors follow `GetWorktreeSettings()` pattern (load config, defaults on error).

- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `feat(workspace): [workspace] root + [[repos]] manifest in config.toml`

### Task A2: Feature-rooted worktree path helper

**Files:**
- Modify: `internal/git/template.go`
- Test: `internal/git/template_test.go` (append)

- [ ] **Step 1: Failing test**

```go
func TestFeatureWorktreePath(t *testing.T) {
	got := FeatureWorktreePath("/ws", "feat/login flow", "repo-a")
	want := filepath.Join("/ws", "feat-login-flow", "worktrees", "repo-a")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
```

- [ ] **Step 2: FAIL** — `go test ./internal/git/ -run TestFeatureWorktreePath -v`
- [ ] **Step 3: Implement**

```go
// FeatureWorktreePath places a worktree under the managed workspace root:
// <root>/<feature-slug>/worktrees/<repo-name>. Feature name is sanitized with
// the same rules as branch path components.
func FeatureWorktreePath(workspaceRoot, feature, repoName string) string {
	return filepath.Join(workspaceRoot, sanitizeBranchForPath(feature), "worktrees", repoName)
}

// FeatureDir returns <root>/<feature-slug> for a feature's docs/metadata.
func FeatureDir(workspaceRoot, feature string) string {
	return filepath.Join(workspaceRoot, sanitizeBranchForPath(feature))
}
```

- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `feat(workspace): FeatureWorktreePath placement helper`

### Task A3: Reroute single-repo worktree placement

**Files:**
- Modify: `internal/ui/worktree_target.go:39-46`
- Test: `internal/ui/worktree_target_test.go`

- [ ] **Step 1: Failing test** — real temp git repo, workspace config set (via temp HOME + config.toml as in A1), call `resolveWorktreeTarget(repo, "feat/x", true)`, assert path == `<root>/feat-x/worktrees/<repoName>`.
- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement** — in `resolveWorktreeTarget`, before the existing logic:

```go
	if ws := session.GetWorkspaceSettings(); ws.Enabled() {
		worktreePath = git.FeatureWorktreePath(ws.RootDir(), branch, filepath.Base(root))
		return worktreePath, root, false, ""
	}
```

(placed after `root` resolution; legacy `[worktree]` path kept as else-branch). Feature name defaults to branch (slug) per spec.

- [ ] **Step 4: PASS** (also run existing tests: `go test ./internal/ui/ -run Worktree -v`)
- [ ] **Step 5: Commit** `feat(workspace): route single-repo worktrees under workspace root`

### Task A4: Reroute multi-repo parent dir

**Files:**
- Modify: `internal/ui/home.go:9202` (worktree branch path) — `parentDir` becomes `<root>/<branch-slug>/worktrees` when workspace enabled; no-worktree path (9230) and fork (9726) keep temp-dir behavior (no branch → no feature).
- Test: covered by B round-trip tests; manual check.

- [ ] **Step 1: Implement**

```go
				parentDir := filepath.Join(home, ".agent-deck", "multi-repo-worktrees",
					fmt.Sprintf("%s-%s", sanitizedBranch, inst.ID[:8]))
				if ws := session.GetWorkspaceSettings(); ws.Enabled() {
					parentDir = filepath.Join(git.FeatureDir(ws.RootDir(), worktreeBranch), "worktrees")
				}
```

- [ ] **Step 2: Build** `go build ./...`
- [ ] **Step 3: Commit** `feat(workspace): multi-repo worktrees under workspace root`

---

## Sub-project B — Feature entity + lifecycle

### Task B1: Schema v11 — features tables + instances.feature_id

**Files:**
- Modify: `internal/statedb/statedb.go` (SchemaVersion 10→11; Migrate(); InstanceRow + scan/insert sites)
- Create: `internal/statedb/features.go`
- Test: `internal/statedb/features_test.go`

- [ ] **Step 1: Failing test** — open in-memory StateDB, Migrate, SaveFeature/GetFeatureByName/ListFeatures/SetFeatureState/DeleteFeature round-trip incl. repos.

```go
func TestFeatureCRUD(t *testing.T) {
	db := openTestDB(t) // existing helper or NewStateDB(":memory:")
	f := statedb.FeatureRow{ID: "f1", Name: "login", State: "active",
		RootPath: "/ws/login", Conductor: false}
	repos := []statedb.FeatureRepoRow{{FeatureID: "f1", RepoName: "alpha",
		RepoPath: "/r/alpha", Branch: "login", BaseRef: "origin/main",
		WorktreePath: "/ws/login/worktrees/alpha"}}
	if err := db.SaveFeature(f, repos); err != nil { t.Fatal(err) }
	got, gotRepos, err := db.GetFeatureByName("login")
	// assert fields; then SetFeatureState("f1","parked"); DeleteFeature("f1") leaves none
}
```

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement**
  - `SchemaVersion = 11`, doc comment for v11.
  - In `Migrate()`: `CREATE TABLE IF NOT EXISTS features (id TEXT PRIMARY KEY, name TEXT UNIQUE NOT NULL, state TEXT NOT NULL DEFAULT 'active', root_path TEXT NOT NULL, conductor INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`; `CREATE TABLE IF NOT EXISTS feature_repos (feature_id TEXT NOT NULL REFERENCES features(id), repo_name TEXT NOT NULL, repo_path TEXT NOT NULL, branch TEXT NOT NULL, base_ref TEXT NOT NULL DEFAULT '', worktree_path TEXT NOT NULL, PRIMARY KEY(feature_id, repo_name))`.
  - alterMigrations += `"ALTER TABLE instances ADD COLUMN feature_id TEXT NOT NULL DEFAULT ''"`; matching column in CREATE TABLE instances; `oldVer < 11` block per existing pattern.
  - `InstanceRow.FeatureID string` + wire into every SELECT/INSERT column list (grep `was_open` to find all sites).
  - `features.go`: FeatureRow/FeatureRepoRow structs + SaveFeature (upsert, tx: delete+insert repos), GetFeatureByName, GetFeatureByID, ListFeatures, SetFeatureState, DeleteFeature (tx: repos then row).
- [ ] **Step 4: PASS** `go test ./internal/statedb/ -v`
- [ ] **Step 5: Commit** `feat(workspace): schema v11 — features tables + instances.feature_id`

### Task B2: Feature service — register/park/resume/delete

**Files:**
- Create: `internal/session/feature.go`
- Test: `internal/session/feature_test.go` (real temp git repos)

- [ ] **Step 1: Failing tests** — behavior-level:

```go
// helper: makeRepo(t) -> init temp git repo with one commit
func TestParkRemovesWorktreesKeepsDocs(t *testing.T) { /* create feature with 1 repo via
	RegisterFeature + real CreateWorktreeAtStartPoint; write docs/handoff.md; Park;
	assert worktree dir gone, branch still exists (git branch --list), docs survive,
	state parked */ }
func TestResumeRecreatesWorktrees(t *testing.T) { /* after park, Resume; assert
	worktree dir back on same branch, state active */ }
func TestParkRefusesDirty(t *testing.T) { /* dirty file in worktree; Park without
	force errors; with force succeeds */ }
```

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement `internal/session/feature.go`**

```go
type FeatureRepo struct {
	RepoName, RepoPath, Branch, BaseRef, WorktreePath string
}

// RegisterFeature persists a feature row + repo snapshot. Idempotent on name.
func RegisterFeature(db *statedb.StateDB, name, rootPath string, conductor bool, repos []FeatureRepo) (featureID string, err error)

// ParkFeature kills nothing (caller stops sessions); removes each worktree via
// git.RemoveWorktree(repoPath, worktreePath, force) after dirty-check
// (git.HasUncommittedChanges or `git -C wt status --porcelain`), refuses if
// CWD inside a target worktree or feature root is a symlink; sets state parked.
func ParkFeature(db *statedb.StateDB, name string, force bool) error

// ResumeFeature recreates each worktree from its existing branch
// (git.CreateWorktree(repoPath, worktreePath, branch) — branch exists → plain
// `worktree add`), sets state active.
func ResumeFeature(db *statedb.StateDB, name string) error

// DeleteFeature parks (force) then removes the feature dir and rows.
func DeleteFeature(db *statedb.StateDB, name string) error
```

Safety order in ParkFeature: symlink check → CWD check → per-repo dirty check (all-or-nothing: collect errors before removing anything) → remove.

- [ ] **Step 4: PASS** `go test ./internal/session/ -run Feature -v`
- [ ] **Step 5: Commit** `feat(workspace): feature service — register/park/resume/delete`

### Task B3: Auto-register feature on worktree session creation

**Files:**
- Modify: `internal/session/instance.go` (add `FeatureID string` json field; map to InstanceRow in todb/fromdb converters)
- Modify: `internal/ui/home.go` — single-repo worktree creation site (after worktree created, ~5984 flow) and multi-repo site (~9212): when workspace enabled, RegisterFeature(branch-slug, featureDir, false, repos…) and set `inst.FeatureID`.
- Test: extend `internal/session/feature_test.go` for RegisterFeature idempotency (same name twice → same ID, repos replaced).

- [ ] Steps: failing test → FAIL → implement → PASS → build → Commit `feat(workspace): auto-register feature for worktree sessions`

### Task B4: Park/resume UX — TUI hotkeys + CLI

**Files:**
- Modify: `internal/ui/home.go` — group/feature context: hotkey `P` park (stops feature's sessions via existing kill flow, then ParkFeature), `R` resume on parked; parked badge in session/group rendering (follow existing badge pattern).
- Create: `cmd/agent-deck/feature_cmd.go` — `agent-deck feature list|park <name> [--force]|resume <name>|delete <name>`; wire into main.go command switch (follow watcher_cmd.go pattern).
- Test: CLI-level smoke via service tests (already covered); manual TUI checklist.

- [ ] Steps: implement CLI first (testable), build, commit `feat(workspace): feature CLI — list/park/resume/delete`; then TUI hotkeys, build, manual check, commit `feat(workspace): TUI park/resume hotkeys + parked badge`.

---

## Sub-project C — Extend

### Task C1: Prereq fix — applyMultiRepoPathChanges must not symlink repos in worktree sessions

**Files:**
- Modify: `internal/ui/home.go:8589` (applyMultiRepoPathChanges)
- Test: `internal/session/multi_repo_worktree_test.go` pattern reused; UI-level logic extracted to a testable helper in `internal/session` if practical.

- [ ] For worktree sessions (`current.WorktreeBranch != ""`): added git-repo paths go through `CreateMultiRepoWorktrees` semantics (real worktree, fail-loud), not `os.Symlink`. Non-git dirs still symlink. Removed paths: `git.RemoveWorktree`.
- [ ] Commit `fix(multi-repo): edit-paths creates real worktrees in worktree sessions`

### Task C2: Extend feature with repo+branch

**Files:**
- Modify: `internal/session/feature.go` — `ExtendFeature(db, name string, repo RepoDef, branch string) error`: create worktree at `FeatureWorktreePath`, append feature_repos row, fail-loud + rollback.
- Modify: `cmd/agent-deck/feature_cmd.go` — `agent-deck feature extend <name> <repo> [--branch X]` (repo = manifest name).
- Modify: `internal/ui/home.go` — EditPathsDialog path: when session has FeatureID, route adds through ExtendFeature; regenerate CLAUDE.md context (`session.ApplyMultiRepoClaudeContext`); restart session.
- Test: feature_test.go — extend adds real worktree (not symlink), single→multi promotion keeps existing worktree.

- [ ] TDD steps as above; commits: `feat(workspace): extend feature with repo+branch (service+CLI)`, `feat(workspace): TUI extend via edit-paths`

---

## Sub-project D — Docs scaffolding + conductor

### Task D1: Feature docs templates

**Files:**
- Create: `internal/session/feature_scaffold.go` (+ embedded templates)
- Test: `internal/session/feature_scaffold_test.go` (render snapshot)

- [ ] `ScaffoldFeatureDocs(featureDir, name string, repos []FeatureRepo, conductor bool) error` — writes `CLAUDE.md` (feature name, repo/branch table, layout note), `docs/handoff.md` stub; conductor adds `contracts/.gitkeep`, `workers/<repo>.md` briefs. Never overwrites existing files (park/resume safe).
- [ ] Call from RegisterFeature (B3 site). Commit `feat(workspace): scaffold feature docs (CLAUDE.md, handoff, contracts, briefs)`

### Task D2: Conductor binding

**Files:**
- Modify: `internal/session/feature.go` + conductor creation path (`internal/session/conductor.go`)

- [ ] Conductor-mode feature start creates conductor session with feature dir as ProjectPath, FeatureID set. Reuse existing conductor infra; no new conductor concepts.
- [ ] Commit `feat(workspace): conductor mode for features`

---

## Verification (whole plan)

- `go build ./...` clean; `go vet ./...` clean.
- Focused suites: `go test ./internal/git/ ./internal/statedb/ ./internal/session/ ./internal/ui/ -run 'Workspace|Feature|Worktree' -v`
- Manual: create worktree session in TUI with `[workspace]` set → dirs under root; park → worktrees gone, docs stay; resume → back; extend → second repo appears as real worktree.
