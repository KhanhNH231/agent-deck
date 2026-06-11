package session

import (
	"os"
	"os/exec"
	"path/filepath"
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
