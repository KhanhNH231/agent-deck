package ui

// E3: TUI per-repo branch overrides.
//
// Dialog state tests — no TTY needed, all state-level assertions.
// Run with: go test ./internal/ui/ -run TestNewDialog_BranchOverride -race -count=1

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
	tea "github.com/charmbracelet/bubbletea"
)

// helpers —————————————————————————————————————————————————————————————————

// makeMinimalGitRepo creates a real git repo (via `git init`) in a temp dir.
func makeMinimalGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func dialogWithMultiRepo(t *testing.T, paths []string) *NewDialog {
	t.Helper()
	d := NewNewDialog()
	d.Show()
	d.multiRepoEnabled = true
	d.multiRepoPaths = append([]string{}, paths...)
	d.multiRepoPathCursor = 0
	d.multiRepoEditing = false
	d.worktreeEnabled = true
	d.branchInput.SetValue("feature/main")
	d.rebuildFocusTargets()
	// Move focus to focusMultiRepo.
	for d.currentTarget() != focusMultiRepo {
		d.focusIndex++
		if d.focusIndex >= len(d.focusTargets) {
			t.Fatal("focusMultiRepo not reachable")
		}
	}
	d.updateFocus()
	return d
}

// ───────── GetMultiRepoBranches ──────────────────────────────────────────────

// TestBranchOverride_GetMultiRepoBranches_NoOverrides verifies that with zero
// overrides the output is value-identical to UniformBranches.
func TestBranchOverride_GetMultiRepoBranches_NoOverrides(t *testing.T) {
	paths := []string{"/repo/a", "/repo/b", "/repo/c"}
	d := NewNewDialog()
	d.multiRepoEnabled = true
	d.multiRepoPaths = append([]string{}, paths...)

	got := d.GetMultiRepoBranches("feat/x", paths)
	want := session.UniformBranches(paths, "feat/x")

	for k, v := range want {
		if got[k] != v {
			t.Errorf("no-override: got[%q]=%q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("len(got)=%d, len(want)=%d", len(got), len(want))
	}
}

// TestBranchOverride_GetMultiRepoBranches_OneOverride: one repo gets a different
// branch; others stay on mainBranch.
func TestBranchOverride_GetMultiRepoBranches_OneOverride(t *testing.T) {
	pathA := "/repo/a"
	pathB := "/repo/b"
	paths := []string{pathA, pathB}

	d := NewNewDialog()
	d.multiRepoEnabled = true
	d.multiRepoPaths = append([]string{}, paths...)
	d.multiRepoBranchOverrides = map[string]string{
		pathA: "custom/branch-a",
	}

	got := d.GetMultiRepoBranches("feat/main", paths)

	if got[pathA] != "custom/branch-a" {
		t.Errorf("pathA: got %q, want %q", got[pathA], "custom/branch-a")
	}
	if got[pathB] != "feat/main" {
		t.Errorf("pathB: got %q, want %q", got[pathB], "feat/main")
	}
}

// TestBranchOverride_GetMultiRepoBranches_AllOverridden: all repos have custom
// branches.
func TestBranchOverride_GetMultiRepoBranches_AllOverridden(t *testing.T) {
	pathA := "/repo/a"
	pathB := "/repo/b"
	paths := []string{pathA, pathB}

	d := NewNewDialog()
	d.multiRepoEnabled = true
	d.multiRepoPaths = append([]string{}, paths...)
	d.multiRepoBranchOverrides = map[string]string{
		pathA: "branch-a",
		pathB: "branch-b",
	}

	got := d.GetMultiRepoBranches("feat/main", paths)

	if got[pathA] != "branch-a" {
		t.Errorf("pathA: got %q, want %q", got[pathA], "branch-a")
	}
	if got[pathB] != "branch-b" {
		t.Errorf("pathB: got %q, want %q", got[pathB], "branch-b")
	}
}

// ───────── Override set / clear-on-equal ─────────────────────────────────────

// TestBranchOverride_SetOverride sets a branch override for a path.
func TestBranchOverride_SetOverride(t *testing.T) {
	d := NewNewDialog()
	d.multiRepoBranchOverrides = make(map[string]string)
	d.setRepoBranchOverride("/repo/a", "custom/a", "feat/main")

	if got, ok := d.multiRepoBranchOverrides["/repo/a"]; !ok || got != "custom/a" {
		t.Errorf("override: got %q ok=%v, want %q true", got, ok, "custom/a")
	}
}

// TestBranchOverride_ClearOnEqual: saving an override equal to mainBranch removes
// the entry.
func TestBranchOverride_ClearOnEqual(t *testing.T) {
	d := NewNewDialog()
	d.multiRepoBranchOverrides = map[string]string{"/repo/a": "old/branch"}

	d.setRepoBranchOverride("/repo/a", "feat/main", "feat/main")

	if _, ok := d.multiRepoBranchOverrides["/repo/a"]; ok {
		t.Error("override equal to mainBranch should be cleared (deleted from map)")
	}
}

// TestBranchOverride_UpdateExisting: setting a different branch on an already-
// overridden path updates the value.
func TestBranchOverride_UpdateExisting(t *testing.T) {
	d := NewNewDialog()
	d.multiRepoBranchOverrides = map[string]string{"/repo/a": "old/branch"}

	d.setRepoBranchOverride("/repo/a", "new/branch", "feat/main")

	if got := d.multiRepoBranchOverrides["/repo/a"]; got != "new/branch" {
		t.Errorf("got %q, want %q", got, "new/branch")
	}
}

// ───────── Non-git row rejects 'b' ───────────────────────────────────────────

// TestBranchOverride_NonGitRow_BKeyShowsHint: pressing 'b' on a non-git row must
// NOT open the inline input; instead a status hint is shown.
func TestBranchOverride_NonGitRow_BKeyShowsHint(t *testing.T) {
	nonGitDir := t.TempDir() // plain dir, no .git
	d := dialogWithMultiRepo(t, []string{nonGitDir})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

	if d.repoOverrideActive {
		t.Error("inline branch input must NOT open for a non-git row")
	}
	// A status hint must be visible.
	if d.validationErr == "" {
		t.Error("a status hint should be set when 'b' is pressed on a non-git row")
	}
	if !strings.Contains(d.validationErr, "git") {
		t.Errorf("hint should mention 'git', got: %q", d.validationErr)
	}
}

// ───────── Git row opens inline input ────────────────────────────────────────

// TestBranchOverride_GitRow_BKeyOpensInput: pressing 'b' on a git-repo row opens
// the inline branch input prefilled with the main branch value.
func TestBranchOverride_GitRow_BKeyOpensInput(t *testing.T) {
	repo := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repo})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

	if !d.repoOverrideActive {
		t.Fatal("inline branch input should open for a git-repo row")
	}
	if d.repoOverridePath != repo {
		t.Errorf("repoOverridePath = %q, want %q", d.repoOverridePath, repo)
	}
	// Prefilled with main branch.
	if got := d.repoOverrideInput.Value(); got != "feature/main" {
		t.Errorf("override input prefilled = %q, want %q", got, "feature/main")
	}
}

// TestBranchOverride_GitRow_EscCancels: pressing ESC while the override input
// is open cancels without saving.
func TestBranchOverride_GitRow_EscCancels(t *testing.T) {
	repo := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repo})

	// Open input.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if !d.repoOverrideActive {
		t.Fatal("precondition: inline input must be open")
	}
	// ESC must cancel.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if d.repoOverrideActive {
		t.Error("repoOverrideActive should be false after ESC")
	}
	if _, ok := d.multiRepoBranchOverrides[repo]; ok {
		t.Error("no override should be saved after ESC")
	}
}

// TestBranchOverride_GitRow_EnterSavesOverride: typing a custom branch and
// pressing Enter saves the override.
func TestBranchOverride_GitRow_EnterSavesOverride(t *testing.T) {
	repo := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repo})

	// Open input.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if !d.repoOverrideActive {
		t.Fatal("precondition: inline input must be open")
	}
	// Replace value with a custom branch.
	d.repoOverrideInput.SetValue("custom/branch-x")
	// Confirm.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if d.repoOverrideActive {
		t.Error("repoOverrideActive should be false after Enter")
	}
	if got, ok := d.multiRepoBranchOverrides[repo]; !ok || got != "custom/branch-x" {
		t.Errorf("override = %q ok=%v, want %q true", got, ok, "custom/branch-x")
	}
}

// TestBranchOverride_GitRow_EnterEqualToMain_ClearsOverride: confirming with
// the same value as the main branch removes the override.
func TestBranchOverride_GitRow_EnterEqualToMain_ClearsOverride(t *testing.T) {
	repo := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repo})

	// Pre-set an existing override.
	if d.multiRepoBranchOverrides == nil {
		d.multiRepoBranchOverrides = make(map[string]string)
	}
	d.multiRepoBranchOverrides[repo] = "old/branch"

	// Open input (prefilled with "feature/main").
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	// Do NOT change the value (it's already "feature/main" = mainBranch).
	// Confirm.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if _, ok := d.multiRepoBranchOverrides[repo]; ok {
		t.Error("override equal to main branch should be cleared on Enter")
	}
}

// ───────── Render suffix ─────────────────────────────────────────────────────

// TestBranchOverride_View_RendersSuffix: rows with an active override render
// an "@<branch>" suffix in the path list.
func TestBranchOverride_View_RendersSuffix(t *testing.T) {
	repo := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repo})
	d.SetSize(120, 50)
	d.multiRepoBranchOverrides = map[string]string{repo: "custom/branch"}

	view := d.View()
	if !strings.Contains(view, "@custom/branch") {
		t.Errorf("view should contain '@custom/branch' suffix for overridden repo; got view excerpt")
	}
}

// ───────── ShowInGroup resets state ──────────────────────────────────────────

func TestBranchOverride_ShowInGroup_Resets(t *testing.T) {
	d := NewNewDialog()
	d.multiRepoBranchOverrides = map[string]string{"/repo/a": "some/branch"}
	d.repoOverrideActive = true
	d.repoOverridePath = "/repo/a"

	d.ShowInGroup("default", "default", "", nil, "")

	if len(d.multiRepoBranchOverrides) != 0 {
		t.Errorf("multiRepoBranchOverrides should be empty after ShowInGroup, got %v", d.multiRepoBranchOverrides)
	}
	if d.repoOverrideActive {
		t.Error("repoOverrideActive should be false after ShowInGroup")
	}
	if d.repoOverridePath != "" {
		t.Errorf("repoOverridePath should be empty after ShowInGroup, got %q", d.repoOverridePath)
	}
}

// ───────── isTextInputFocused gate ───────────────────────────────────────────

// TestBranchOverride_IsTextInputFocused_WhenOverrideActive: isTextInputFocused
// must return true while the override input is open so letter shortcuts don't
// fire.
func TestBranchOverride_IsTextInputFocused_WhenOverrideActive(t *testing.T) {
	repo := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repo})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if !d.repoOverrideActive {
		t.Fatal("precondition: override input must be open")
	}

	if !d.isTextInputFocused() {
		t.Error("isTextInputFocused() must return true while repoOverrideActive")
	}
}

// TestBranchOverride_LetterKeys_TypeIntoOverrideInput: typing while the override
// input is open appends to the input, not triggers other shortcuts (e.g. 'a'
// adds a path, 'd' deletes, etc.).
func TestBranchOverride_LetterKeys_TypeIntoOverrideInput(t *testing.T) {
	repo := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repo})
	// Add a second path so 'd' would have something to delete.
	d.multiRepoPaths = append(d.multiRepoPaths, "/repo/other")

	// Open override input.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	// Clear prefilled value so typing starts fresh.
	d.repoOverrideInput.SetValue("")

	// Type 'a' and 'd' — must go to text input, not trigger path add/delete.
	countBefore := len(d.multiRepoPaths)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if len(d.multiRepoPaths) != countBefore {
		t.Errorf("path count changed (%d→%d) while override input was open; letters leaked to shortcuts",
			countBefore, len(d.multiRepoPaths))
	}
}

// ───────── Lifecycle: delete / edit must clean up overrides ──────────────────

// TestBranchOverride_DeletePath_ClearsOverride: deleting a path row with 'd'
// must drop its override; re-adding the same path must NOT resurrect the old
// override.
func TestBranchOverride_DeletePath_ClearsOverride(t *testing.T) {
	repoA := makeMinimalGitRepo(t)
	repoB := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repoA, repoB})

	// Set an override on repoA (cursor 0) via the 'b' flow.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if !d.repoOverrideActive {
		t.Fatal("precondition: override input must open for git repo")
	}
	d.repoOverrideInput.SetValue("custom/a")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := d.multiRepoBranchOverrides[repoA]; got != "custom/a" {
		t.Fatalf("precondition: override = %q, want custom/a", got)
	}

	// Delete the repoA row (cursor still 0; len > 1 so 'd' is allowed).
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(d.multiRepoPaths) != 1 || d.multiRepoPaths[0] != repoB {
		t.Fatalf("precondition: delete failed, paths = %v", d.multiRepoPaths)
	}

	// Re-add the same path.
	d.multiRepoPaths = append(d.multiRepoPaths, repoA)

	// The old override must NOT resurface in the submit map.
	branches := d.GetMultiRepoBranches("feature/main", []string{repoB, repoA})
	if got := branches[repoA]; got != "feature/main" {
		t.Errorf("re-added path branch = %q, want %q (stale override resurfaced)", got, "feature/main")
	}
}

// TestBranchOverride_EditPath_ClearsOldOverride: editing a path in-place must
// drop the OLD key's override — the new path starts on the main branch.
func TestBranchOverride_EditPath_ClearsOldOverride(t *testing.T) {
	repoA := makeMinimalGitRepo(t)
	repoNew := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repoA})

	// Set an override on repoA.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if !d.repoOverrideActive {
		t.Fatal("precondition: override input must open for git repo")
	}
	d.repoOverrideInput.SetValue("custom/a")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Edit row 0 to point at a different repo: Enter starts editing,
	// replace the path, Enter accepts.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !d.multiRepoEditing {
		t.Fatal("precondition: enter should start path editing")
	}
	d.pathInput.SetValue(repoNew)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.multiRepoEditing {
		t.Fatal("precondition: enter should accept the edit")
	}

	// Old key's override must be gone.
	if _, ok := d.multiRepoBranchOverrides[repoA]; ok {
		t.Error("old path's override must be removed when the path is edited")
	}
	// New path gets the main branch (no inherited override).
	branches := d.GetMultiRepoBranches("feature/main", []string{repoNew})
	if got := branches[repoNew]; got != "feature/main" {
		t.Errorf("edited path branch = %q, want %q", got, "feature/main")
	}
}

// TestBranchOverride_EditPath_SameValue_KeepsOverride: accepting an edit that
// leaves the path unchanged must NOT drop the override.
func TestBranchOverride_EditPath_SameValue_KeepsOverride(t *testing.T) {
	repoA := makeMinimalGitRepo(t)
	d := dialogWithMultiRepo(t, []string{repoA})

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	d.repoOverrideInput.SetValue("custom/a")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Start editing and accept without changing the value.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if got := d.multiRepoBranchOverrides[repoA]; got != "custom/a" {
		t.Errorf("no-op edit dropped the override: got %q, want custom/a", got)
	}
}

// ───────── Keying parity with GetMultiRepoPaths ──────────────────────────────

// TestBranchOverride_MalformedTildePath_KeySurvivesToSubmitMap pins keying
// parity between the override map and GetMultiRepoPaths (which builds allPaths
// at the home.go submit site). GetMultiRepoPaths applies a mid-string "~/"
// repair (textinput suggestion-append artifact, same as GetValues) BEFORE
// ExpandPath; the override key must be normalized identically or the override
// silently falls back to the main branch — the silent default the E3 spec bans.
func TestBranchOverride_MalformedTildePath_KeySurvivesToSubmitMap(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempHome)
	defer os.Setenv("HOME", origHome)
	session.ClearUserConfigCache()
	defer session.ClearUserConfigCache()

	// Real git repo at ~/repo.
	repoDir := filepath.Join(tempHome, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if out, err := exec.Command("git", "-C", repoDir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	otherRepo := makeMinimalGitRepo(t)

	// Malformed row value: cwd prefix glued to a "~/" path (the artifact
	// GetMultiRepoPaths repairs via strings.Index(p, "~/") > 0).
	malformed := "/stale-suggestion-prefix~/repo"
	d := dialogWithMultiRepo(t, []string{malformed, otherRepo})

	// 'b' on the malformed row must normalize the same way as submit:
	// detect ~/repo → $HOME/repo → a git repo → input opens.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if !d.repoOverrideActive {
		t.Fatalf("override input did not open: keying normalization diverges from GetMultiRepoPaths (validationErr=%q)", d.validationErr)
	}
	d.repoOverrideInput.SetValue("custom/tilde")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Submit-site simulation: same calls home.go makes.
	allPaths, enabled := d.GetMultiRepoPaths()
	if !enabled || len(allPaths) != 2 {
		t.Fatalf("GetMultiRepoPaths = %v enabled=%v, want 2 paths", allPaths, enabled)
	}
	branches := d.GetMultiRepoBranches("feature/main", allPaths)

	expanded := filepath.Join(tempHome, "repo")
	if got := branches[expanded]; got != "custom/tilde" {
		t.Errorf("override lost between dialog and submit map: branches[%q] = %q, want custom/tilde (map=%v, overrides=%v)",
			expanded, got, branches, d.multiRepoBranchOverrides)
	}
	if got := branches[otherRepo]; got != "feature/main" {
		t.Errorf("non-overridden repo = %q, want feature/main", got)
	}
}
