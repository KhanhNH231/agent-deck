package git

// Tests that remote fetches are bounded by fetchTimeout (E5 follow-up).
// A hung remote must not block the calling goroutine forever — under E5's
// unattended background ticks that would be a goroutine leak.
//
// The hang is simulated with git's ext:: transport: the remote helper command
// is `sleep 60`, which never speaks the git protocol, so the fetch blocks
// until killed. No network involved.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupHangingRemoteRepo creates a repo with one seed commit and a remote
// named "hang" whose URL is an ext:: transport that sleeps forever.
func setupHangingRemoteRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	mustGitE1(t, repo, "init", "-b", "main")
	mustGitE1(t, repo, "config", "user.email", "test@test.com")
	mustGitE1(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	mustGitE1(t, repo, "add", ".")
	mustGitE1(t, repo, "commit", "-m", "seed")

	// ext:: transport runs `sleep 60` as the remote helper — it never
	// responds, so the fetch hangs until the context deadline kills git.
	mustGitE1(t, repo, "remote", "add", "hang", "ext::sleep 60")
	mustGitE1(t, repo, "config", "protocol.ext.allow", "always")
	return repo
}

// TestFetchRemoteShapedRef_TimeoutOnHungRemote verifies that a hung remote
// produces a warning within ~fetchTimeout instead of blocking forever.
func TestFetchRemoteShapedRef_TimeoutOnHungRemote(t *testing.T) {
	repo := setupHangingRemoteRepo(t)

	old := fetchTimeout
	fetchTimeout = 1 * time.Second
	t.Cleanup(func() { fetchTimeout = old })

	start := time.Now()
	warning := fetchRemoteShapedRef(repo, "hang/main")
	elapsed := time.Since(start)

	if warning == "" {
		t.Fatal("expected non-empty warning from hung-remote fetch, got \"\"")
	}
	// 1s timeout + process-kill overhead; anything beyond 5s means the
	// deadline did not bound the fetch.
	if elapsed > 5*time.Second {
		t.Fatalf("fetch took %v, expected to be killed by ~1s fetchTimeout", elapsed)
	}
}

// TestFreshOriginDefaultBranchRef_TimeoutOnHungRemote verifies the second
// fetch site: a hung default remote returns ok=false within the deadline
// (callers fall back to a HEAD-based branch).
func TestFreshOriginDefaultBranchRef_TimeoutOnHungRemote(t *testing.T) {
	repo := t.TempDir()
	mustGitE1(t, repo, "init", "-b", "main")
	mustGitE1(t, repo, "config", "user.email", "test@test.com")
	mustGitE1(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	mustGitE1(t, repo, "add", ".")
	mustGitE1(t, repo, "commit", "-m", "seed")

	// "origin" is the default remote; point it at the hanging ext transport
	// and pin the default branch so GetDefaultBranch resolves without a remote
	// round-trip.
	mustGitE1(t, repo, "remote", "add", "origin", "ext::sleep 60")
	mustGitE1(t, repo, "config", "protocol.ext.allow", "always")
	mustGitE1(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	old := fetchTimeout
	fetchTimeout = 1 * time.Second
	t.Cleanup(func() { fetchTimeout = old })

	start := time.Now()
	ref, ok := freshOriginDefaultBranchRef(repo)
	elapsed := time.Since(start)

	if ok {
		t.Fatalf("expected ok=false from hung-remote fetch, got ref=%q", ref)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("fetch took %v, expected to be killed by ~1s fetchTimeout", elapsed)
	}
}
