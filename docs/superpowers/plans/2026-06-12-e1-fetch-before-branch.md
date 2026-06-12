# E1: Fetch Before Branch Create — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every worktree-creation path branches from a freshly-fetched ref instead of a stale local remote-tracking ref; fetch failure warns and proceeds (offline-friendly).

**Architecture:** One new unexported helper `fetchRemoteShapedRef` in `internal/git/git.go` (mirrors the existing `freshOriginDefaultBranchRef` / #973 precedent). Wired into three arms: `CreateWorktreeAtStartPoint` (gains a `warning` return, threaded to CLI stderr via `ExtendFeature`), and `CreateWorktree`'s remote-tracking + existing-local arms (silent best-effort, no signature change — too many callers).

**Tech Stack:** Go 1.24, table of real temp git repos with `file://` remotes (pattern: `internal/git/issue973_worker_spawn_fresh_main_test.go`).

**Spec:** `docs/superpowers/specs/2026-06-12-branch-features-design.md` (E1 section)
**Branch:** feat/agent-deck-session-ux (already checked out)

---

## File structure

| File | Change |
|---|---|
| `internal/git/git.go` | Add `fetchRemoteShapedRef` + `branchUpstreamRef`; modify `CreateWorktreeAtStartPoint` (3-value return), `CreateWorktree` remote/local arms |
| `internal/git/branch_freshen_test.go` | New — all E1 behavior tests |
| `internal/session/feature.go` | `ExtendFeature` returns `(warning string, err error)`; `ResumeFeature` untouched (uses `CreateWorktree`) |
| `cmd/agent-deck/feature_cmd.go` | Print extend warning to stderr |
| `cmd/agent-deck/session_cmd.go` | Mechanical: absorb new return value (SHA start point → warning always empty) |

5 prod files but 2 are mechanical one-liners; core logic is git.go only.

---

## Task 1: `fetchRemoteShapedRef` + CreateWorktreeAtStartPoint freshness (TDD)

**Files:**
- Modify: `internal/git/git.go` (helper near `freshOriginDefaultBranchRef` ~line 779; `CreateWorktreeAtStartPoint` at line 446)
- Create: `internal/git/branch_freshen_test.go`
- Modify: `cmd/agent-deck/session_cmd.go:859`, `internal/session/feature.go:198` (mechanical compile fixes ONLY in this task — semantic threading is Task 2)

- [ ] **Step 1: Write the failing behavior test.** Read `internal/git/issue973_worker_spawn_fresh_main_test.go` first and reuse its origin/clone/advance helpers style. New file `internal/git/branch_freshen_test.go`:

```go
package git

// Tests E1 (branch-features spec): worktree creation fetches remote-shaped
// refs before branching, and degrades to a warning when the remote is
// unreachable. Repo topology mirrors issue973_worker_spawn_fresh_main_test.go:
// bare origin + two clones; cloneB advances origin after cloneA's initial fetch,
// so cloneA's remote-tracking ref is genuinely stale.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateWorktreeAtStartPoint_FetchesRemoteShapedStartPoint(t *testing.T) {
	origin, cloneA, cloneB := setupStaleTrackingRepos(t) // helper below
	_ = origin

	// Advance origin's master via cloneB AFTER cloneA last fetched.
	advancedTip := advanceBranch(t, cloneB, "master", "remote-advance.txt")

	wt := filepath.Join(t.TempDir(), "wt")
	created, warning, err := CreateWorktreeAtStartPoint(cloneA, wt, "e1-feature", "origin/master")
	if err != nil {
		t.Fatalf("CreateWorktreeAtStartPoint: %v", err)
	}
	if !created {
		t.Fatal("expected createdBranch=true")
	}
	if warning != "" {
		t.Fatalf("expected no warning with reachable remote, got %q", warning)
	}
	head, err := HeadCommit(wt)
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if head != advancedTip {
		t.Fatalf("worktree rooted at stale ref: head=%s want fresh origin tip %s", head, advancedTip)
	}
}

func TestCreateWorktreeAtStartPoint_UnreachableRemoteWarnsAndProceeds(t *testing.T) {
	_, cloneA, _ := setupStaleTrackingRepos(t)

	// Break the remote so fetch must fail.
	mustGit(t, cloneA, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

	wt := filepath.Join(t.TempDir(), "wt")
	created, warning, err := CreateWorktreeAtStartPoint(cloneA, wt, "e1-offline", "origin/master")
	if err != nil {
		t.Fatalf("offline create must succeed with stale ref, got: %v", err)
	}
	if !created {
		t.Fatal("expected createdBranch=true")
	}
	if warning == "" || !strings.Contains(warning, "origin") {
		t.Fatalf("expected fetch-failure warning naming the remote, got %q", warning)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("worktree dir missing: %v", err)
	}
}

func TestCreateWorktreeAtStartPoint_NonRemoteStartPointNoFetchNoWarning(t *testing.T) {
	_, cloneA, _ := setupStaleTrackingRepos(t)
	sha, err := HeadCommit(cloneA)
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	created, warning, err := CreateWorktreeAtStartPoint(cloneA, wt, "e1-sha", sha)
	if err != nil || !created {
		t.Fatalf("SHA start point: created=%v err=%v", created, err)
	}
	if warning != "" {
		t.Fatalf("SHA start point must not warn, got %q", warning)
	}
}
```

Plus the shared helpers in the same file (adapt exact git plumbing from the issue973 test — author commits with `-c user.email/-c user.name` flags the way that file does):

```go
// setupStaleTrackingRepos: bare origin with one commit on master, cloneA and
// cloneB cloned from it. cloneA has fetched once; later advances via cloneB
// make cloneA's origin/master stale.
func setupStaleTrackingRepos(t *testing.T) (origin, cloneA, cloneB string) { ... }

// advanceBranch commits a file on branch in repo and pushes; returns new tip SHA.
func advanceBranch(t *testing.T, repo, branch, filename string) string { ... }

// mustGit runs git -C dir args... failing the test on error.
func mustGit(t *testing.T, dir string, args ...string) { ... }
```

- [ ] **Step 2: Run, verify compile-level RED.**

Run: `go test ./internal/git/ -run 'CreateWorktreeAtStartPoint_Fetches|CreateWorktreeAtStartPoint_Unreachable|CreateWorktreeAtStartPoint_NonRemote' -race -count=1`
Expected: FAIL — compile error (`CreateWorktreeAtStartPoint` returns 2 values, tests expect 3).

- [ ] **Step 3: Implement.** In `internal/git/git.go`:

(a) Helper, placed right after `freshOriginDefaultBranchRef` (~line 793):

```go
// fetchRemoteShapedRef best-effort-fetches ref when it has the shape
// <remote>/<branch> for a configured remote. Returns "" when the fetch
// succeeded or ref is not remote-shaped (local branch, SHA, tag); returns a
// human-readable warning when the fetch failed — callers proceed with the
// stale ref (offline work must not block; branch-features spec E1).
func fetchRemoteShapedRef(repoDir, ref string) string {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	remote, branch := parts[0], parts[1]
	remotes, err := listRemotes(repoDir)
	if err != nil {
		return ""
	}
	found := false
	for _, r := range remotes {
		if r == remote {
			found = true
			break
		}
	}
	if !found {
		return ""
	}
	fetch := exec.Command("git", "-C", repoDir, "fetch", "--quiet", remote, branch)
	if err := fetch.Run(); err != nil {
		return fmt.Sprintf("fetch %s %s failed, branching from possibly-stale ref %s", remote, branch, ref)
	}
	return ""
}
```

(b) `CreateWorktreeAtStartPoint` — new signature and fetch call:

```go
func CreateWorktreeAtStartPoint(repoDir, worktreePath, branchName, startPoint string) (createdBranch bool, warning string, err error) {
	if err := ValidateBranchName(branchName); err != nil {
		return false, "", fmt.Errorf("invalid branch name: %w", err)
	}
	if strings.TrimSpace(startPoint) == "" {
		return false, "", errors.New("start point cannot be empty")
	}
	repoDir = resolveGitInvocationDir(repoDir)
	if !IsGitRepo(repoDir) {
		return false, "", errors.New("not a git repository")
	}
	if BranchExists(repoDir, branchName) {
		return false, "", fmt.Errorf("branch %q already exists", branchName)
	}
	warning = fetchRemoteShapedRef(repoDir, startPoint)
	cmd := exec.Command("git", "-C", repoDir, "worktree", "add", "-b", branchName, worktreePath, startPoint)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, warning, fmt.Errorf("failed to create worktree at start point: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return true, warning, nil
}
```

(c) Mechanical compile fixes (semantics in Task 2):
- `cmd/agent-deck/session_cmd.go:859`: `createdBranch, cwErr := ...` → `createdBranch, _, cwErr := ...` (start point is a SHA — `fetchRemoteShapedRef` no-ops; discard).
- `internal/session/feature.go:198`: `if _, err := git.CreateWorktreeAtStartPoint(...)` → `if _, _, err := ...` (warning threading is Task 2).

- [ ] **Step 4: Existing tests for the old signature.** `internal/git/git_test.go:1274,1347` (`TestCreateWorktreeAtStartPoint_*`) destructure 2 values — update them to 3 (`created, _, err`), changing nothing else.

- [ ] **Step 5: Run, verify GREEN.**

Run: `go test ./internal/git/ -run 'CreateWorktreeAtStartPoint' -race -count=1 -timeout 120s`
Expected: PASS (all five: 2 pre-existing + 3 new).

- [ ] **Step 6: Build whole module:** `go build ./...` — must compile clean.

- [ ] **Step 7: Commit**

```bash
git add internal/git/git.go internal/git/branch_freshen_test.go internal/git/git_test.go cmd/agent-deck/session_cmd.go internal/session/feature.go
git commit -m "feat(git): fetch remote-shaped start points before worktree create (E1)

Co-Authored-By: Claude"
```

---

## Task 2: Thread extend warning to CLI stderr (TDD)

**Files:**
- Modify: `internal/session/feature.go` (`ExtendFeature` ~line 171)
- Modify: `cmd/agent-deck/feature_cmd.go` (extend handler ~line 99)
- Test: extend an existing test in `internal/session/feature_test.go` (find the `ExtendFeature` test; add a broken-remote case)

- [ ] **Step 1: Failing test.** In `internal/session/feature_test.go`, locate the existing `ExtendFeature` happy-path test and its repo fixtures. Add:

```go
func TestExtendFeature_UnreachableRemoteWarnsAndProceeds(t *testing.T) {
	// Arrange exactly like the existing ExtendFeature test fixture, but break
	// the new repo's origin before extending:
	//   mustGit(t, newRepo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	// (reuse this package's git-helper conventions)
	warning, err := session.ExtendFeature(db, featureName, repoName, newRepo, "origin/master")
	if err != nil {
		t.Fatalf("extend must proceed offline: %v", err)
	}
	if warning == "" {
		t.Fatal("expected stale-ref warning from unreachable remote")
	}
	// worktree must still exist (assert same way the happy-path test does)
}
```

Adapt arrange/assert details to the real fixture (the implementer reads the existing test first; if `ExtendFeature` is tested through a different public surface, hang the new case off that same surface).

- [ ] **Step 2: RED run.** `go test ./internal/session/ -run 'ExtendFeature' -race -count=1` — compile FAIL (single return today).

- [ ] **Step 3: Implement.**
- `feature.go`: `func ExtendFeature(...) error` → `(warning string, err error)`. Inside, capture from Task 1's signature: `created, warning, err := git.CreateWorktreeAtStartPoint(repoRoot, wtPath, branch, baseRef)` and return `warning` alongside the existing returns (all early-error returns become `return "", fmt.Errorf(...)`).
- `feature_cmd.go` extend handler:
```go
warning, err := session.ExtendFeature(db, name, repoName, repoPath, baseRef)
if err != nil { /* existing error path unchanged */ }
if warning != "" {
	fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
}
```
- Fix any other `ExtendFeature` callers the compiler reports (expected: only the CLI).

- [ ] **Step 4: GREEN run.** `go test ./internal/session/ -run 'ExtendFeature' -race -count=1 -timeout 120s` → PASS. `go build ./...` → clean.

- [ ] **Step 5: Commit**

```bash
git add internal/session/feature.go internal/session/feature_test.go cmd/agent-deck/feature_cmd.go
git commit -m "feat(feature): surface stale-ref warning on extend (E1)

Co-Authored-By: Claude"
```

---

## Task 3: Best-effort freshen in CreateWorktree remote + local arms (TDD)

**Files:**
- Modify: `internal/git/git.go` (`CreateWorktree` switch ~line 399; new helper `branchUpstreamRef`)
- Modify: `internal/git/branch_freshen_test.go` (add 2 tests)

Silent best-effort (no signature change — `CreateWorktree` has many callers; mirrors the #973 precedent where offline falls through silently).

- [ ] **Step 1: Failing tests** (append to `branch_freshen_test.go`):

```go
func TestCreateWorktree_ExistingRemoteBranch_FetchesBeforeTracking(t *testing.T) {
	_, cloneA, cloneB := setupStaleTrackingRepos(t)

	// Branch exists ONLY on remote: create + push from cloneB, then advance it.
	mustGit(t, cloneB, "checkout", "-b", "shared-feat")
	advanceBranch(t, cloneB, "shared-feat", "first.txt")
	advancedTip := advanceBranch(t, cloneB, "shared-feat", "second.txt")
	// cloneA fetches once so the remote branch is KNOWN but its tracking ref
	// goes stale after one more advance from cloneB:
	mustGit(t, cloneA, "fetch", "origin")
	finalTip := advanceBranch(t, cloneB, "shared-feat", "third.txt")
	_ = advancedTip

	wt := filepath.Join(t.TempDir(), "wt")
	if err := CreateWorktree(cloneA, wt, "shared-feat"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	head, _ := HeadCommit(wt)
	if head != finalTip {
		t.Fatalf("tracked stale remote ref: head=%s want %s", head, finalTip)
	}
}

func TestCreateWorktree_ExistingLocalBranch_FreshensUpstreamTrackingRef(t *testing.T) {
	_, cloneA, cloneB := setupStaleTrackingRepos(t)

	// Local branch with upstream: master in cloneA tracks origin/master.
	// Advance origin AFTER cloneA's clone-time fetch.
	freshTip := advanceBranch(t, cloneB, "master", "advance.txt")

	wt := filepath.Join(t.TempDir(), "wt")
	// master is checked out in cloneA; worktree-add of a checked-out branch
	// fails, so use a second local branch with upstream instead:
	mustGit(t, cloneA, "branch", "--track", "local-feat", "origin/master")
	if err := CreateWorktree(cloneA, wt, "local-feat"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	// Checkout stays at the LOCAL tip (ff is E4's job), but the remote-tracking
	// ref must now know the fresh remote tip:
	out := gitOutput(t, cloneA, "rev-parse", "origin/master") // helper: git -C + trim
	if out != freshTip {
		t.Fatalf("tracking ref still stale: %s want %s", out, freshTip)
	}
}
```

- [ ] **Step 2: RED run.** `go test ./internal/git/ -run 'TestCreateWorktree_Existing' -race -count=1` — both FAIL (no fetch happens today; remote-arm head lands on the stale tip, local-arm tracking ref stays stale).

- [ ] **Step 3: Implement** in `CreateWorktree`'s switch (git.go:399):

```go
	case worktreeBranchLocal:
		// Freshen the branch's upstream tracking ref so divergence is visible
		// immediately (checkout stays at the local tip — ff-pull is E4).
		if upstream, ok := branchUpstreamRef(repoDir, branchName); ok {
			_ = fetchRemoteShapedRef(repoDir, upstream)
		}
		cmd = exec.Command("git", "-C", repoDir, "worktree", "add", worktreePath, branchName)
	case worktreeBranchRemote:
		remoteRef := resolution.Remote + "/" + branchName
		// Best-effort freshen (mirrors #973): a stale remote-tracking ref would
		// root the new local branch at an old tip. Offline falls through.
		_ = fetchRemoteShapedRef(repoDir, remoteRef)
		cmd = exec.Command("git", "-C", repoDir, "worktree", "add", "--track", "-b", branchName, worktreePath, remoteRef)
```

New helper next to `fetchRemoteShapedRef`:

```go
// branchUpstreamRef returns the upstream tracking ref (e.g. "origin/main") of
// a local branch, ok=false when the branch has no upstream configured.
func branchUpstreamRef(repoDir, branch string) (string, bool) {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "--abbrev-ref", branch+"@{upstream}")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	ref := strings.TrimSpace(string(output))
	if ref == "" {
		return "", false
	}
	return ref, true
}
```

- [ ] **Step 4: GREEN + regression sweep.**

Run: `go test ./internal/git/ -race -count=1 -timeout 300s`
Expected: full package PASS (covers #973 + worktree suites — fetch additions must not break offline-style existing tests; they use local-only repos where `listRemotes` is empty → helper no-ops).

- [ ] **Step 5: Build:** `go build ./...` → clean.

- [ ] **Step 6: Commit**

```bash
git add internal/git/git.go internal/git/branch_freshen_test.go
git commit -m "feat(git): freshen remote refs in CreateWorktree remote/local arms (E1)

Co-Authored-By: Claude"
```

---

## Self-review notes

- **Spec coverage:** start (TUI/CLI use `CreateWorktree`/multi-repo paths → Task 3 remote+local arms; new-branch arm already fresh via #973), extend (Task 1+2), resume (recreates from existing branches → Task 3 local/remote arms). Warn-and-proceed: Task 1 (warning return) + Task 2 (stderr); CreateWorktree arms intentionally silent — documented deviation, consistent with #973 precedent and avoids a many-caller signature change. ✓
- **Type consistency:** `CreateWorktreeAtStartPoint` 3-return used identically in Tasks 1–2; `fetchRemoteShapedRef(repoDir, ref) string` same in Tasks 1+3. ✓
- **No placeholders:** helper bodies for the test file are signature+contract level by design — implementer adapts the issue973 plumbing it must read first; all production code is complete. ✓
- **Known risk:** multi-repo creation calls `CreateWorktree` per repo → one fetch per repo per feature start. Acceptable (interactive flow); E5 will need rate-limiting, not E1.
- **`feature.go:198` exact destructure may drift** if E2 lands first — plan assumes E1 before E2 per spec build order.
