package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asheshgoplani/agent-deck/internal/git"
	"github.com/asheshgoplani/agent-deck/internal/statedb"
)

// FeatureRepo describes one repo participating in a feature. Mirrors
// statedb.FeatureRepoRow without the FeatureID back-reference so callers can
// build the set before the feature exists.
type FeatureRepo struct {
	RepoName     string
	RepoPath     string
	Branch       string
	BaseRef      string
	WorktreePath string
}

// FeatureStateActive / FeatureStateParked are the feature lifecycle states.
const (
	FeatureStateActive = "active"
	FeatureStateParked = "parked"
)

// RegisterFeature persists a feature row plus its repo snapshot. Idempotent
// on name: re-registering an existing feature keeps its ID, replaces the repo
// snapshot, and re-activates it.
func RegisterFeature(db *statedb.StateDB, name, rootPath string, conductor bool, repos []FeatureRepo) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("feature name must not be empty")
	}
	id := GenerateID()
	if existing, _, err := db.GetFeatureByName(name); err == nil {
		id = existing.ID
	}
	rows := make([]statedb.FeatureRepoRow, 0, len(repos))
	for _, r := range repos {
		rows = append(rows, statedb.FeatureRepoRow{
			FeatureID:    id,
			RepoName:     r.RepoName,
			RepoPath:     r.RepoPath,
			Branch:       r.Branch,
			BaseRef:      r.BaseRef,
			WorktreePath: r.WorktreePath,
		})
	}
	err := db.SaveFeature(statedb.FeatureRow{
		ID:        id,
		Name:      name,
		State:     FeatureStateActive,
		RootPath:  rootPath,
		Conductor: conductor,
	}, rows)
	if err != nil {
		return "", fmt.Errorf("register feature %q: %w", name, err)
	}
	return id, nil
}

// RegisterWorktreeSessionFeature registers (or refreshes) the feature backing
// a worktree session under the managed workspace root. The feature is named
// after the branch. Returns "" without error when the workspace is disabled
// or db is nil, so call sites can wire it unconditionally.
func RegisterWorktreeSessionFeature(db *statedb.StateDB, branch string, repos []FeatureRepo) (string, error) {
	ws := GetWorkspaceSettings()
	if db == nil || !ws.Enabled() || branch == "" || len(repos) == 0 {
		return "", nil
	}
	featureDir := git.FeatureDir(ws.RootDir(), branch)
	id, err := RegisterFeature(db, branch, featureDir, false, repos)
	if err != nil {
		return "", err
	}
	// Best-effort doc skeleton; never overwrites user edits, so safe on
	// re-register (extend, resume, repeat sessions on the same branch).
	if scErr := ScaffoldFeatureDocs(featureDir, branch, repos, false); scErr != nil {
		return id, scErr
	}
	return id, nil
}

// ParkFeature removes a feature's worktrees while keeping its docs and
// branches, then marks it parked. Safety gates, all checked BEFORE anything
// is removed (all-or-nothing):
//   - feature root must not be a symlink
//   - the current working directory must not be inside a target worktree
//   - every worktree must be clean unless force is set
func ParkFeature(db *statedb.StateDB, name string, force bool) error {
	f, repos, err := db.GetFeatureByName(name)
	if err != nil {
		return fmt.Errorf("park: %w", err)
	}

	if fi, err := os.Lstat(f.RootPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("park %q: feature root %s is a symlink — refusing", name, f.RootPath)
	}
	cwd, _ := os.Getwd()
	var problems []string
	for _, r := range repos {
		if r.WorktreePath == "" {
			continue
		}
		if cwd != "" && pathWithin(cwd, r.WorktreePath) {
			problems = append(problems, fmt.Sprintf("cwd is inside %s", r.WorktreePath))
			continue
		}
		if _, err := os.Stat(r.WorktreePath); os.IsNotExist(err) {
			continue // already gone — parking is idempotent
		}
		if !force {
			dirty, err := git.HasUncommittedChanges(r.WorktreePath)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: dirty-check failed: %v", r.RepoName, err))
			} else if dirty {
				problems = append(problems, fmt.Sprintf("%s has uncommitted changes (use force)", r.RepoName))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("park %q refused: %s", name, strings.Join(problems, "; "))
	}

	for _, r := range repos {
		if r.WorktreePath == "" {
			continue
		}
		if _, err := os.Stat(r.WorktreePath); os.IsNotExist(err) {
			continue
		}
		if err := git.RemoveWorktree(r.RepoPath, r.WorktreePath, force); err != nil {
			return fmt.Errorf("park %q: remove worktree %s: %w", name, r.RepoName, err)
		}
	}

	return db.SetFeatureState(f.ID, FeatureStateParked)
}

// ResumeFeature recreates a parked feature's worktrees from their existing
// branches and marks it active. Worktrees that already exist are kept as-is.
func ResumeFeature(db *statedb.StateDB, name string) error {
	f, repos, err := db.GetFeatureByName(name)
	if err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	for _, r := range repos {
		if r.WorktreePath == "" {
			continue
		}
		if _, err := os.Stat(r.WorktreePath); err == nil {
			continue
		}
		// The branch survived park, so CreateWorktree takes its existing-branch
		// path (plain `git worktree add <path> <branch>`).
		if err := git.CreateWorktree(r.RepoPath, r.WorktreePath, r.Branch); err != nil {
			return fmt.Errorf("resume %q: worktree %s: %w", name, r.RepoName, err)
		}
	}
	return db.SetFeatureState(f.ID, FeatureStateActive)
}

// ExtendFeature adds a repo to an existing feature: creates a REAL worktree
// for it under <feature-root>/worktrees/<repoName> on the feature's branch
// (created from baseRef when given, e.g. "origin/master") and appends the
// repo to the feature's snapshot. Fails loudly; never symlinks a live repo.
func ExtendFeature(db *statedb.StateDB, name, repoName, repoPath, baseRef string) error {
	f, repos, err := db.GetFeatureByName(name)
	if err != nil {
		return fmt.Errorf("extend: %w", err)
	}
	for _, r := range repos {
		if r.RepoName == repoName || r.RepoPath == repoPath {
			return fmt.Errorf("extend %q: repo %s already part of the feature", name, repoName)
		}
	}
	branch := name
	if len(repos) > 0 && repos[0].Branch != "" {
		branch = repos[0].Branch
	}
	if !git.IsGitRepoOrBareProjectRoot(repoPath) {
		return fmt.Errorf("extend %q: %s is not a git repository", name, repoPath)
	}
	repoRoot, err := git.GetWorktreeBaseRoot(repoPath)
	if err != nil {
		return fmt.Errorf("extend %q: resolve repo root: %w", name, err)
	}

	wtPath := filepath.Join(f.RootPath, "worktrees", repoName)
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return fmt.Errorf("extend %q: %w", name, err)
	}
	if baseRef != "" {
		if _, err := git.CreateWorktreeAtStartPoint(repoRoot, wtPath, branch, baseRef); err != nil {
			return fmt.Errorf("extend %q: worktree %s: %w", name, repoName, err)
		}
	} else {
		if err := git.CreateWorktree(repoRoot, wtPath, branch); err != nil {
			return fmt.Errorf("extend %q: worktree %s: %w", name, repoName, err)
		}
	}

	newRepos := make([]FeatureRepo, 0, len(repos)+1)
	for _, r := range repos {
		newRepos = append(newRepos, FeatureRepo{
			RepoName: r.RepoName, RepoPath: r.RepoPath, Branch: r.Branch,
			BaseRef: r.BaseRef, WorktreePath: r.WorktreePath,
		})
	}
	newRepos = append(newRepos, FeatureRepo{
		RepoName: repoName, RepoPath: repoRoot, Branch: branch,
		BaseRef: baseRef, WorktreePath: wtPath,
	})
	if _, err := RegisterFeature(db, name, f.RootPath, f.Conductor, newRepos); err != nil {
		// The worktree exists but the snapshot update failed — roll it back
		// so disk and DB stay consistent.
		_ = git.RemoveWorktree(repoRoot, wtPath, true)
		return err
	}
	return nil
}

// MarkFeatureConductor flags a feature as conductor-orchestrated and
// (re-)scaffolds its conductor docs: contracts/ and per-repo worker briefs.
// Returns the feature row for callers that wire up the conductor session.
func MarkFeatureConductor(db *statedb.StateDB, name string) (statedb.FeatureRow, error) {
	f, repoRows, err := db.GetFeatureByName(name)
	if err != nil {
		return statedb.FeatureRow{}, fmt.Errorf("conductor: %w", err)
	}
	repos := make([]FeatureRepo, 0, len(repoRows))
	for _, r := range repoRows {
		repos = append(repos, FeatureRepo{
			RepoName: r.RepoName, RepoPath: r.RepoPath, Branch: r.Branch,
			BaseRef: r.BaseRef, WorktreePath: r.WorktreePath,
		})
	}
	f.Conductor = true
	if err := db.SaveFeature(f, repoRows); err != nil {
		return statedb.FeatureRow{}, fmt.Errorf("conductor: %w", err)
	}
	if err := ScaffoldFeatureDocs(f.RootPath, name, repos, true); err != nil {
		return statedb.FeatureRow{}, err
	}
	return f, nil
}

// DeleteFeature force-parks the feature, removes its directory (docs
// included), and deletes its rows. Branches stay in their repos.
func DeleteFeature(db *statedb.StateDB, name string) error {
	f, _, err := db.GetFeatureByName(name)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if err := ParkFeature(db, name, true); err != nil {
		return err
	}
	if f.RootPath != "" {
		if err := os.RemoveAll(f.RootPath); err != nil {
			return fmt.Errorf("delete %q: remove %s: %w", name, f.RootPath, err)
		}
	}
	return db.DeleteFeature(f.ID)
}

// pathWithin reports whether path is dir or inside dir.
func pathWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
