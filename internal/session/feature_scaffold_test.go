package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldFeatureDocs(t *testing.T) {
	dir := t.TempDir()
	repos := []FeatureRepo{
		{RepoName: "alpha", RepoPath: "/r/alpha", Branch: "feat/x", BaseRef: "origin/main"},
		{RepoName: "beta", RepoPath: "/r/beta", Branch: "feat/x"},
	}

	if err := ScaffoldFeatureDocs(dir, "feat/x", repos, false); err != nil {
		t.Fatalf("ScaffoldFeatureDocs: %v", err)
	}

	claude, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md missing: %v", err)
	}
	for _, want := range []string{"feat/x", "alpha", "beta", "worktrees/"} {
		if !strings.Contains(string(claude), want) {
			t.Fatalf("CLAUDE.md missing %q:\n%s", want, claude)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "handoff.md")); err != nil {
		t.Fatalf("handoff.md missing: %v", err)
	}
	// Non-conductor: no contracts/workers.
	if _, err := os.Stat(filepath.Join(dir, "contracts")); !os.IsNotExist(err) {
		t.Fatal("contracts/ should not exist without conductor")
	}
}

func TestScaffoldFeatureDocsConductorAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	repos := []FeatureRepo{{RepoName: "alpha", RepoPath: "/r/alpha", Branch: "feat/x"}}

	if err := ScaffoldFeatureDocs(dir, "feat/x", repos, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "contracts")); err != nil {
		t.Fatalf("contracts/ missing in conductor mode: %v", err)
	}
	brief := filepath.Join(dir, "workers", "alpha.md")
	if _, err := os.Stat(brief); err != nil {
		t.Fatalf("worker brief missing: %v", err)
	}

	// Re-scaffold must never overwrite user edits.
	handoff := filepath.Join(dir, "docs", "handoff.md")
	if err := os.WriteFile(handoff, []byte("user notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ScaffoldFeatureDocs(dir, "feat/x", repos, true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(handoff)
	if string(got) != "user notes" {
		t.Fatalf("handoff.md overwritten: %q", got)
	}
}
