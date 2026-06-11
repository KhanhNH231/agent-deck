package session

import (
	"os"
	"path/filepath"
	"testing"
)

func writeWorkspaceConfig(t *testing.T, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := filepath.Join(home, ".agent-deck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ClearUserConfigCache()
	t.Cleanup(ClearUserConfigCache)
}

func TestWorkspaceSettingsFromTOML(t *testing.T) {
	writeWorkspaceConfig(t, `
[workspace]
root = "~/ws"

[[repos]]
name = "alpha"
path = "~/repos/alpha"
default_base = "origin/main"

[[repos]]
name = "beta"
path = "~/repos/beta"
`)
	home, _ := os.UserHomeDir()

	ws := GetWorkspaceSettings()
	if !ws.Enabled() {
		t.Fatal("workspace should be enabled when root set")
	}
	if got, want := ws.RootDir(), filepath.Join(home, "ws"); got != want {
		t.Fatalf("RootDir = %q, want %q", got, want)
	}

	repo, ok := FindManifestRepo("alpha")
	if !ok {
		t.Fatal("alpha not found in manifest")
	}
	if got, want := repo.PathExpanded(), filepath.Join(home, "repos", "alpha"); got != want {
		t.Fatalf("PathExpanded = %q, want %q", got, want)
	}
	if repo.DefaultBase != "origin/main" {
		t.Fatalf("DefaultBase = %q", repo.DefaultBase)
	}

	if got := len(GetManifestRepos()); got != 2 {
		t.Fatalf("manifest repos = %d, want 2", got)
	}
	if _, ok := FindManifestRepo("missing"); ok {
		t.Fatal("missing repo should not be found")
	}
}

func TestWorkspaceSettingsAbsent(t *testing.T) {
	writeWorkspaceConfig(t, `
[worktree]
default_location = "sibling"
`)
	if GetWorkspaceSettings().Enabled() {
		t.Fatal("workspace must be disabled when section absent")
	}
	if got := len(GetManifestRepos()); got != 0 {
		t.Fatalf("manifest repos = %d, want 0", got)
	}
}
