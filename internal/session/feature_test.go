package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/git"
	"github.com/asheshgoplani/agent-deck/internal/statedb"
)

func initFeatureTestRepo(t *testing.T, name string) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-c", "init.defaultBranch=main", "init"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func openFeatureTestDB(t *testing.T) *statedb.StateDB {
	t.Helper()
	db, err := statedb.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// startTestFeature registers a feature with one real worktree and returns
// (featureID, featureDir, repoDir, worktreePath).
func startTestFeature(t *testing.T, db *statedb.StateDB, name string) (string, string, string, string) {
	t.Helper()
	repo := initFeatureTestRepo(t, "alpha")
	wsRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	featureDir := git.FeatureDir(wsRoot, name)
	wtPath := git.FeatureWorktreePath(wsRoot, name, "alpha")
	if err := git.CreateWorktree(repo, wtPath, name); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	id, err := RegisterFeature(db, name, featureDir, false, []FeatureRepo{{
		RepoName: "alpha", RepoPath: repo, Branch: name, WorktreePath: wtPath,
	}})
	if err != nil {
		t.Fatalf("RegisterFeature: %v", err)
	}
	return id, featureDir, repo, wtPath
}

func TestRegisterFeatureIdempotent(t *testing.T) {
	db := openFeatureTestDB(t)
	id1, _, repo, wt := startTestFeature(t, db, "login")
	id2, err := RegisterFeature(db, "login", filepath.Dir(filepath.Dir(wt)), false, []FeatureRepo{{
		RepoName: "alpha", RepoPath: repo, Branch: "login", WorktreePath: wt,
	}})
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("ids differ: %q vs %q", id1, id2)
	}
}

func TestParkRemovesWorktreesKeepsDocs(t *testing.T) {
	db := openFeatureTestDB(t)
	_, featureDir, repo, wtPath := startTestFeature(t, db, "login")

	docs := filepath.Join(featureDir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "handoff.md"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ParkFeature(db, "login", false); err != nil {
		t.Fatalf("ParkFeature: %v", err)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(docs, "handoff.md")); err != nil {
		t.Fatalf("docs must survive park: %v", err)
	}
	out, err := exec.Command("git", "-C", repo, "branch", "--list", "login").Output()
	if err != nil || len(out) == 0 {
		t.Fatalf("branch must survive park: %v %q", err, out)
	}
	f, _, err := db.GetFeatureByName("login")
	if err != nil || f.State != "parked" {
		t.Fatalf("state = %+v err=%v", f, err)
	}
}

func TestParkRefusesDirtyUnlessForced(t *testing.T) {
	db := openFeatureTestDB(t)
	_, _, _, wtPath := startTestFeature(t, db, "login")

	if err := os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ParkFeature(db, "login", false); err == nil {
		t.Fatal("expected dirty-worktree refusal")
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree must be untouched after refusal: %v", err)
	}
	if err := ParkFeature(db, "login", true); err != nil {
		t.Fatalf("forced park: %v", err)
	}
}

func TestResumeRecreatesWorktrees(t *testing.T) {
	db := openFeatureTestDB(t)
	_, _, repo, wtPath := startTestFeature(t, db, "login")

	// Commit a marker on the feature branch so resume provably checks out
	// the same branch state.
	if err := os.WriteFile(filepath.Join(wtPath, "work.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "wip"},
	} {
		cmd := exec.Command("git", append([]string{"-C", wtPath}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if err := ParkFeature(db, "login", false); err != nil {
		t.Fatalf("ParkFeature: %v", err)
	}
	if err := ResumeFeature(db, "login"); err != nil {
		t.Fatalf("ResumeFeature: %v", err)
	}

	if _, err := os.Stat(filepath.Join(wtPath, "work.txt")); err != nil {
		t.Fatalf("resumed worktree missing branch content: %v", err)
	}
	f, _, err := db.GetFeatureByName("login")
	if err != nil || f.State != "active" {
		t.Fatalf("state = %+v err=%v", f, err)
	}
	_ = repo
}

func TestDeleteFeatureRemovesDirAndRows(t *testing.T) {
	db := openFeatureTestDB(t)
	_, featureDir, _, _ := startTestFeature(t, db, "login")

	if err := DeleteFeature(db, "login"); err != nil {
		t.Fatalf("DeleteFeature: %v", err)
	}
	if _, err := os.Stat(featureDir); !os.IsNotExist(err) {
		t.Fatalf("feature dir should be gone: %v", err)
	}
	if _, _, err := db.GetFeatureByName("login"); err == nil {
		t.Fatal("feature row should be gone")
	}
}

func TestRegisterWorktreeSessionFeature(t *testing.T) {
	db := openFeatureTestDB(t)
	writeWorkspaceConfig(t, "[workspace]\nroot = \"~/ws\"\n")

	id, err := RegisterWorktreeSessionFeature(db, "feat/x", []FeatureRepo{{
		RepoName: "alpha", RepoPath: "/r/alpha", Branch: "feat/x", WorktreePath: "/ws/feat-x/worktrees/alpha",
	}})
	if err != nil || id == "" {
		t.Fatalf("expected registration, got id=%q err=%v", id, err)
	}
	f, _, err := db.GetFeatureByName("feat/x")
	if err != nil || f.State != FeatureStateActive {
		t.Fatalf("feature = %+v err=%v", f, err)
	}
}

func TestRegisterWorktreeSessionFeatureNoopWhenDisabled(t *testing.T) {
	db := openFeatureTestDB(t)
	writeWorkspaceConfig(t, "")
	id, err := RegisterWorktreeSessionFeature(db, "feat/x", []FeatureRepo{{RepoName: "a"}})
	if err != nil || id != "" {
		t.Fatalf("expected noop, got id=%q err=%v", id, err)
	}
}

func TestExtendFeatureAddsRealWorktree(t *testing.T) {
	db := openFeatureTestDB(t)
	_, featureDir, _, _ := startTestFeature(t, db, "login")
	repoB := initFeatureTestRepo(t, "beta")

	if _, err := ExtendFeature(db, "login", "beta", repoB, "", "login"); err != nil {
		t.Fatalf("ExtendFeature: %v", err)
	}

	_, repos, err := db.GetFeatureByName("login")
	if err != nil || len(repos) != 2 {
		t.Fatalf("repos = %+v err=%v", repos, err)
	}
	wtB := filepath.Join(featureDir, "worktrees", "beta")
	fi, err := os.Lstat(wtB)
	if err != nil {
		t.Fatalf("beta worktree missing: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("beta worktree is a symlink — must be real")
	}
	out := gitOut(t, repoB, "branch", "--list", "login")
	if len(strings.TrimSpace(out)) == 0 {
		t.Fatal("branch login not created in beta")
	}
}

func TestExtendFeatureRejectsDuplicateRepo(t *testing.T) {
	db := openFeatureTestDB(t)
	_, _, repoA, _ := startTestFeature(t, db, "login")
	if _, err := ExtendFeature(db, "login", "alpha", repoA, "", "login"); err == nil {
		t.Fatal("expected duplicate-repo rejection")
	}
}

// setupRepoWithRemote creates a bare origin, clones it into repoName, seeds one
// commit, pushes, so the clone has a real origin/<branch> tracking ref.
// Returns (cloneDir). The remote URL is within tmp so it can be broken by
// set-url to a nonexistent path.
func setupRepoWithRemote(t *testing.T, repoName, branch string) string {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	originDir := filepath.Join(tmp, "origin.git")
	if err := os.MkdirAll(originDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
	}

	mustGit(originDir, "init", "--bare", "-b", branch)

	cloneDir := filepath.Join(tmp, repoName)
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(cloneDir, "init", "-b", branch)
	mustGit(cloneDir, "config", "user.email", "t@t")
	mustGit(cloneDir, "config", "user.name", "t")
	mustGit(cloneDir, "remote", "add", "origin", originDir)
	if err := os.WriteFile(filepath.Join(cloneDir, "README.md"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(cloneDir, "add", ".")
	mustGit(cloneDir, "commit", "-m", "seed")
	mustGit(cloneDir, "push", "-u", "origin", branch)

	return cloneDir
}

func TestExtendFeature_UnreachableRemoteWarnsAndProceeds(t *testing.T) {
	db := openFeatureTestDB(t)
	_, featureDir, _, _ := startTestFeature(t, db, "login")

	// Set up a beta repo that has a real origin + remote-tracking ref for main.
	betaRepo := setupRepoWithRemote(t, "beta", "main")

	// Break the remote so fetch fails (offline simulation).
	cmd := exec.Command("git", "-C", betaRepo, "remote", "set-url", "origin", "/nonexistent/gone.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set-url: %v\n%s", err, out)
	}

	warning, err := ExtendFeature(db, "login", "beta", betaRepo, "origin/main", "login")
	if err != nil {
		t.Fatalf("extend must proceed offline: %v", err)
	}
	if warning == "" {
		t.Fatal("expected stale-ref warning from unreachable remote")
	}

	// Worktree must exist and be a real dir (not a symlink).
	wtB := filepath.Join(featureDir, "worktrees", "beta")
	fi, err := os.Lstat(wtB)
	if err != nil {
		t.Fatalf("beta worktree missing: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("beta worktree is a symlink — must be real")
	}

	// DB snapshot must include beta.
	_, repos, err := db.GetFeatureByName("login")
	if err != nil || len(repos) != 2 {
		t.Fatalf("repos = %+v err=%v", repos, err)
	}
}

// Warning and error coexist legitimately: a typo'd/unfetchable base ref yields
// BOTH a fetch warning AND a worktree-add error — and the warning explains the
// error. ExtendFeature must not drop the warning on the error path.
func TestExtendFeature_WarningSurvivesWorktreeFailure(t *testing.T) {
	db := openFeatureTestDB(t)
	_, _, _, _ = startTestFeature(t, db, "login")

	betaRepo := setupRepoWithRemote(t, "beta", "main")

	// Break the remote so fetch fails (offline simulation).
	cmd := exec.Command("git", "-C", betaRepo, "remote", "set-url", "origin", "/nonexistent/gone.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set-url: %v\n%s", err, out)
	}

	// Base ref has no local remote-tracking ref: fetch fails (warning), then
	// worktree-add cannot resolve the start point (error).
	warning, err := ExtendFeature(db, "login", "beta", betaRepo, "origin/nonexistent-branch", "login")
	if err == nil {
		t.Fatal("expected worktree-add error for unresolvable base ref")
	}
	if warning == "" {
		t.Fatal("expected fetch warning to survive the worktree-add error")
	}
}

// TestExtendFeature_ExplicitBranchDiffersFromFeature: when a branch name that
// differs from the feature name is passed, the repo row must record that branch
// and the created worktree must be checked out on it.
func TestExtendFeature_ExplicitBranchDiffersFromFeature(t *testing.T) {
	db := openFeatureTestDB(t)
	_, featureDir, _, _ := startTestFeature(t, db, "login")
	repoB := initFeatureTestRepo(t, "beta")

	const explicitBranch = "feat/other-line"
	if _, err := ExtendFeature(db, "login", "beta", repoB, "", explicitBranch); err != nil {
		t.Fatalf("ExtendFeature: %v", err)
	}

	_, repos, err := db.GetFeatureByName("login")
	if err != nil {
		t.Fatalf("GetFeatureByName: %v", err)
	}
	var betaRow *statedb.FeatureRepoRow
	for i := range repos {
		if repos[i].RepoName == "beta" {
			betaRow = &repos[i]
		}
	}
	if betaRow == nil {
		t.Fatal("beta repo row not found")
	}
	if betaRow.Branch != explicitBranch {
		t.Fatalf("feature_repos.Branch = %q, want %q", betaRow.Branch, explicitBranch)
	}

	wtB := filepath.Join(featureDir, "worktrees", "beta")
	out, err := exec.Command("git", "-C", wtB, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD in worktree: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != explicitBranch {
		t.Fatalf("worktree HEAD = %q, want %q", got, explicitBranch)
	}
}

// TestExtendFeature_EmptyBranchRejected: passing an empty branch to
// ExtendFeature must return an error immediately; no worktree created, no row
// added to the DB.
func TestExtendFeature_EmptyBranchRejected(t *testing.T) {
	db := openFeatureTestDB(t)
	_, featureDir, _, _ := startTestFeature(t, db, "login")
	repoB := initFeatureTestRepo(t, "beta")

	_, err := ExtendFeature(db, "login", "beta", repoB, "", "")
	if err == nil {
		t.Fatal("expected error for empty branch")
	}

	// No worktree should have been created.
	wtB := filepath.Join(featureDir, "worktrees", "beta")
	if _, statErr := os.Stat(wtB); !os.IsNotExist(statErr) {
		t.Fatalf("worktree must not exist after empty-branch rejection, stat err = %v", statErr)
	}

	// No new repo row should have been added.
	_, repos, err2 := db.GetFeatureByName("login")
	if err2 != nil {
		t.Fatalf("GetFeatureByName: %v", err2)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo row after rejection, got %d", len(repos))
	}
}

// ── UpdateFeature tests ────────────────────────────────────────────────────

// setupRepoWithRemoteAndWorktree sets up a repo with a real upstream and
// creates a worktree at wtPath on a feature branch derived from "main".
// The feature branch has the same initial commit as main and is pushed to
// origin, so it has a remote-tracking ref. Returns (cloneDir).
// When behindByOne is true, a second commit is pushed to origin/<featureBranch>
// so the clone's worktree is 1 behind — the file "remote_change.txt" appears
// in the worktree after an update.
func setupRepoWithRemoteAndWorktree(t *testing.T, repoName, featureBranch, wtPath string, behindByOne bool) string {
	t.Helper()
	// setupRepoWithRemote seeds a clone on "main" with one commit + origin.
	cloneDir := setupRepoWithRemote(t, repoName, "main")

	mustGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, cmdErr, out)
		}
	}

	// Create and push featureBranch from the current HEAD.
	mustGit(cloneDir, "checkout", "-b", featureBranch)
	mustGit(cloneDir, "push", "-u", "origin", featureBranch)
	// Return to main so featureBranch can be put in a worktree.
	mustGit(cloneDir, "checkout", "main")

	if behindByOne {
		// Push an extra commit to origin/<featureBranch> via a second clone.
		originURL, err := exec.Command("git", "-C", cloneDir, "remote", "get-url", "origin").Output()
		if err != nil {
			t.Fatalf("get origin url: %v", err)
		}
		tmp, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		clone2 := filepath.Join(tmp, "clone2")
		mustGit2 := func(dir string, args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
				t.Fatalf("git %v in %s: %v\n%s", args, dir, cmdErr, out)
			}
		}
		mustGit2(tmp, "clone", strings.TrimSpace(string(originURL)), "clone2")
		mustGit2(clone2, "config", "user.email", "t@t")
		mustGit2(clone2, "config", "user.name", "t")
		mustGit2(clone2, "checkout", featureBranch)
		if err := os.WriteFile(filepath.Join(clone2, "remote_change.txt"), []byte("from remote"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit2(clone2, "add", ".")
		mustGit2(clone2, "commit", "-m", "remote commit")
		mustGit2(clone2, "push", "origin", featureBranch)
	}

	// Create the worktree at wtPath from cloneDir on featureBranch.
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", cloneDir, "worktree", "add", wtPath, featureBranch).CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}
	return cloneDir
}

// TestUpdateFeature_MixedOutcomes: feature with 2 repos — repoA behind
// upstream (→ FFUpdated; file from remote materialises in worktree), repoB
// up-to-date (→ FFUpToDate). No error returned.
func TestUpdateFeature_MixedOutcomes(t *testing.T) {
	db := openFeatureTestDB(t)

	wsRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const featureName = "mixed-update"
	const featureBranch = "feat/mixed-update"

	featureDir := git.FeatureDir(wsRoot, featureName)
	wtA := git.FeatureWorktreePath(wsRoot, featureName, "repo-a")
	wtB := git.FeatureWorktreePath(wsRoot, featureName, "repo-b")

	// repoA is 1 commit behind its origin on featureBranch.
	repoA := setupRepoWithRemoteAndWorktree(t, "repo-a", featureBranch, wtA, true)
	// repoB is up-to-date (same commit as origin) on featureBranch.
	repoB := setupRepoWithRemoteAndWorktree(t, "repo-b", featureBranch, wtB, false)

	_, err = RegisterFeature(db, featureName, featureDir, false, []FeatureRepo{
		{RepoName: "repo-a", RepoPath: repoA, Branch: featureBranch, WorktreePath: wtA},
		{RepoName: "repo-b", RepoPath: repoB, Branch: featureBranch, WorktreePath: wtB},
	})
	if err != nil {
		t.Fatalf("RegisterFeature: %v", err)
	}

	updates, err := UpdateFeature(db, featureName)
	if err != nil {
		t.Fatalf("UpdateFeature: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}

	byRepo := make(map[string]FeatureRepoUpdate, 2)
	for _, u := range updates {
		byRepo[u.RepoName] = u
	}

	// repoA must be updated.
	a := byRepo["repo-a"]
	if a.Result.Outcome != git.FFUpdated {
		t.Fatalf("repo-a: expected FFUpdated, got %q (detail: %s)", a.Result.Outcome, a.Result.Detail)
	}
	if a.Result.OldTip == "" || a.Result.NewTip == "" {
		t.Fatalf("repo-a: expected OldTip/NewTip set, got %q/%q", a.Result.OldTip, a.Result.NewTip)
	}
	// The remote commit's file must materialise in the worktree.
	if _, err := os.Stat(filepath.Join(wtA, "remote_change.txt")); err != nil {
		t.Fatalf("remote_change.txt not present in updated worktree: %v", err)
	}

	// repoB must be up-to-date.
	b := byRepo["repo-b"]
	if b.Result.Outcome != git.FFUpToDate {
		t.Fatalf("repo-b: expected FFUpToDate, got %q (detail: %s)", b.Result.Outcome, b.Result.Detail)
	}
}

// TestUpdateFeature_ParkedErrors: parked feature → error mentioning "parked".
func TestUpdateFeature_ParkedErrors(t *testing.T) {
	db := openFeatureTestDB(t)
	_, _, _, _ = startTestFeature(t, db, "login")

	if err := ParkFeature(db, "login", false); err != nil {
		t.Fatalf("ParkFeature: %v", err)
	}

	_, err := UpdateFeature(db, "login")
	if err == nil {
		t.Fatal("expected error for parked feature")
	}
	if !strings.Contains(err.Error(), "parked") {
		t.Fatalf("error must mention 'parked', got: %v", err)
	}
}

// TestUpdateFeature_MissingWorktreeReported: a feature whose WorktreePath was
// deleted from disk → FFMissing outcome for that repo, other repos still
// processed (no error returned).
func TestUpdateFeature_MissingWorktreeReported(t *testing.T) {
	db := openFeatureTestDB(t)

	wsRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const featureName = "missing-wt"
	const featureBranch = "feat/missing-wt"

	featureDir := git.FeatureDir(wsRoot, featureName)
	wtA := git.FeatureWorktreePath(wsRoot, featureName, "repo-a")
	wtB := git.FeatureWorktreePath(wsRoot, featureName, "repo-b")

	// Both repos up-to-date; we will manually delete wtA afterwards.
	repoA := setupRepoWithRemoteAndWorktree(t, "repo-a", featureBranch, wtA, false)
	repoB := setupRepoWithRemoteAndWorktree(t, "repo-b", featureBranch, wtB, false)

	_, err = RegisterFeature(db, featureName, featureDir, false, []FeatureRepo{
		{RepoName: "repo-a", RepoPath: repoA, Branch: featureBranch, WorktreePath: wtA},
		{RepoName: "repo-b", RepoPath: repoB, Branch: featureBranch, WorktreePath: wtB},
	})
	if err != nil {
		t.Fatalf("RegisterFeature: %v", err)
	}

	// Remove wtA from disk (simulate deleted/moved worktree without DB update).
	if out, rmErr := exec.Command("git", "-C", repoA, "worktree", "remove", "--force", wtA).CombinedOutput(); rmErr != nil {
		t.Fatalf("git worktree remove: %v\n%s", rmErr, out)
	}
	if _, statErr := os.Stat(wtA); !os.IsNotExist(statErr) {
		// Fallback: brutal removal.
		_ = os.RemoveAll(wtA)
	}

	updates, err := UpdateFeature(db, featureName)
	if err != nil {
		t.Fatalf("UpdateFeature must not error on missing worktree: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}

	byRepo := make(map[string]FeatureRepoUpdate, 2)
	for _, u := range updates {
		byRepo[u.RepoName] = u
	}

	// repo-a must report FFMissing.
	a := byRepo["repo-a"]
	if a.Result.Outcome != git.FFMissing {
		t.Fatalf("repo-a: expected FFMissing, got %q", a.Result.Outcome)
	}

	// repo-b must still be processed (FFUpToDate expected).
	b := byRepo["repo-b"]
	if b.Result.Outcome != git.FFUpToDate {
		t.Fatalf("repo-b: expected FFUpToDate after missing-wt sibling, got %q (detail: %s)", b.Result.Outcome, b.Result.Detail)
	}
}

// TestFeature_MixedBranchParkResumeRoundTrip verifies that a feature whose two
// repos sit on DIFFERENT branches survives a full park→resume cycle:
//   - ParkFeature removes both worktrees but leaves both branches in their repos
//   - ResumeFeature recreates each worktree on its ORIGINAL per-repo branch
//     (reads feature_repos.branch per row, never a feature-level branch)
func TestFeature_MixedBranchParkResumeRoundTrip(t *testing.T) {
	db := openFeatureTestDB(t)

	// Two independent repos, each starting on main.
	repoA := initFeatureTestRepo(t, "repo-a")
	repoB := initFeatureTestRepo(t, "repo-b")

	// Workspace layout mirrors what startTestFeature / FeatureWorktreePath use.
	wsRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const featureName = "mixed-branch-feature"
	const branchA = "feat/line-a"
	const branchB = "feat/line-b"

	featureDir := git.FeatureDir(wsRoot, featureName)
	wtA := git.FeatureWorktreePath(wsRoot, featureName, "repo-a")
	wtB := git.FeatureWorktreePath(wsRoot, featureName, "repo-b")

	// Create real worktrees on their respective branches.
	if err := git.CreateWorktree(repoA, wtA, branchA); err != nil {
		t.Fatalf("CreateWorktree repo-a: %v", err)
	}
	if err := git.CreateWorktree(repoB, wtB, branchB); err != nil {
		t.Fatalf("CreateWorktree repo-b: %v", err)
	}

	_, err = RegisterFeature(db, featureName, featureDir, false, []FeatureRepo{
		{RepoName: "repo-a", RepoPath: repoA, Branch: branchA, WorktreePath: wtA},
		{RepoName: "repo-b", RepoPath: repoB, Branch: branchB, WorktreePath: wtB},
	})
	if err != nil {
		t.Fatalf("RegisterFeature: %v", err)
	}

	// ── Park ────────────────────────────────────────────────────────────────
	if err := ParkFeature(db, featureName, false); err != nil {
		t.Fatalf("ParkFeature: %v", err)
	}

	// Both worktree dirs must be gone.
	for _, wt := range []string{wtA, wtB} {
		if _, err := os.Stat(wt); !os.IsNotExist(err) {
			t.Fatalf("worktree %s should be gone after park, stat err = %v", wt, err)
		}
	}

	// Both branches must still exist in their origin repos.
	out := gitOut(t, repoA, "rev-parse", "--verify", branchA)
	if strings.TrimSpace(out) == "" {
		t.Fatalf("branch %s must survive park in repo-a", branchA)
	}
	out = gitOut(t, repoB, "rev-parse", "--verify", branchB)
	if strings.TrimSpace(out) == "" {
		t.Fatalf("branch %s must survive park in repo-b", branchB)
	}

	// Feature state must be parked.
	f, _, err := db.GetFeatureByName(featureName)
	if err != nil || f.State != FeatureStateParked {
		t.Fatalf("expected state=parked, got state=%q err=%v", f.State, err)
	}

	// ── Resume ───────────────────────────────────────────────────────────────
	if err := ResumeFeature(db, featureName); err != nil {
		t.Fatalf("ResumeFeature: %v", err)
	}

	// Both worktrees must be back and checked out on their ORIGINAL branches.
	for _, tc := range []struct {
		wt     string
		branch string
	}{
		{wtA, branchA},
		{wtB, branchB},
	} {
		if _, err := os.Stat(tc.wt); err != nil {
			t.Fatalf("worktree %s missing after resume: %v", tc.wt, err)
		}
		out, err := exec.Command("git", "-C", tc.wt, "rev-parse", "--abbrev-ref", "HEAD").Output()
		if err != nil {
			t.Fatalf("rev-parse HEAD in %s: %v", tc.wt, err)
		}
		if got := strings.TrimSpace(string(out)); got != tc.branch {
			t.Fatalf("worktree %s HEAD = %q, want %q", tc.wt, got, tc.branch)
		}
	}

	// Feature state must be active.
	f, _, err = db.GetFeatureByName(featureName)
	if err != nil || f.State != FeatureStateActive {
		t.Fatalf("expected state=active after resume, got state=%q err=%v", f.State, err)
	}
}
