package ui

// Tests for the "Default" model-picker entry in the new-session dialog.
//
// "Default" is a selectable list item (first suggestion) that maps to NO model
// override: choosing it makes GetLaunchModelID() return "" so the agent CLI
// uses its own configured default and ToArgs() emits no --model flag.
//
// It is distinct from the [claude].default_model preselect (#1172): "Default"
// is the explicit no-override choice and overrides any preselected catalog id.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
	tea "github.com/charmbracelet/bubbletea"
)

// focusModelForTool returns a visible dialog with the given tool selected and
// focus parked on the model field (picker showing).
func focusModelForTool(t *testing.T, tool string) *NewDialog {
	t.Helper()
	d := NewNewDialog()
	d.SetDefaultTool(tool)
	d.SetSize(100, 50)
	d.Show()
	d.focusIndex = d.indexOf(focusModel)
	if d.focusIndex < 0 {
		t.Fatalf("tool %q should expose a focusable model field", tool)
	}
	d.updateFocus()
	return d
}

// selectDefaultEntry navigates the picker to the "Default" entry (cursor 1, just
// below the "Type custom" entry at cursor 0) and selects it with Space.
func selectDefaultEntry(t *testing.T, d *NewDialog) *NewDialog {
	t.Helper()
	// Activate the dropdown and move onto the Default entry.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown}) // 0 -> 1 (Default)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	return d
}

// "Default" appears as the first catalog suggestion for every supported tool.
func TestDefaultModel_EntryShownForSupportedTools(t *testing.T) {
	for _, tool := range []string{"claude", "gemini", "opencode", "codex"} {
		t.Run(tool, func(t *testing.T) {
			d := focusModelForTool(t, tool)
			d.filterModelSuggestions()
			if len(d.modelSuggestions) == 0 {
				t.Fatalf("tool %q: no model suggestions", tool)
			}
			if d.modelSuggestions[0] != defaultModelSentinel {
				t.Fatalf("tool %q: first suggestion = %q, want %q", tool, d.modelSuggestions[0], defaultModelSentinel)
			}
		})
	}
}

// Selecting "Default" yields an empty launch model (no --model override).
func TestDefaultModel_SelectingYieldsEmptyLaunchModel(t *testing.T) {
	for _, tool := range []string{"claude", "gemini", "opencode", "codex"} {
		t.Run(tool, func(t *testing.T) {
			d := focusModelForTool(t, tool)
			d = selectDefaultEntry(t, d)
			if got := d.GetLaunchModelID(); got != "" {
				t.Fatalf("tool %q: GetLaunchModelID() = %q after selecting Default, want empty", tool, got)
			}
		})
	}
}

// "Default" must NOT appear for tools without model support (shell, crush).
func TestDefaultModel_NotShownForUnsupportedTools(t *testing.T) {
	for _, tool := range []string{"", "crush"} {
		t.Run("tool="+tool, func(t *testing.T) {
			d := NewNewDialog()
			d.SetDefaultTool(tool)
			d.SetSize(100, 50)
			d.Show()
			d.filterModelSuggestions()
			for _, s := range d.modelSuggestions {
				if s == defaultModelSentinel {
					t.Fatalf("tool %q: Default entry must not be offered for an unsupported tool", tool)
				}
			}
		})
	}
}

// Custom free-text input must still work alongside the Default entry: typing a
// model the catalog doesn't know still flows through to GetLaunchModelID().
func TestDefaultModel_CustomInputStillWorks(t *testing.T) {
	d := focusModelForTool(t, "codex")
	d = typeRunes(d, "qwen3-custom-zzz")
	if got := d.GetLaunchModelID(); got != "qwen3-custom-zzz" {
		t.Fatalf("GetLaunchModelID() = %q, want qwen3-custom-zzz (custom input broke)", got)
	}
}

// Selecting "Default" overrides the #1172 [claude].default_model preselect: the
// dialog opens preselected to the configured model, but choosing Default clears
// it back to empty (no override).
func TestDefaultModel_OverridesIssue1172Preselect(t *testing.T) {
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	t.Cleanup(func() { os.Setenv("HOME", originalHome) })

	session.ClearUserConfigCache()
	t.Cleanup(session.ClearUserConfigCache)

	if err := os.MkdirAll(filepath.Join(tempDir, ".agent-deck"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := session.SaveUserConfig(&session.UserConfig{
		Claude: session.ClaudeSettings{DefaultModel: "claude-opus-4-7"},
	}); err != nil {
		t.Fatalf("SaveUserConfig: %v", err)
	}
	session.ClearUserConfigCache()

	d := NewNewDialog()
	d.SetDefaultTool("claude")
	d.SetSize(100, 50)
	d.ShowInGroup("projects", "Projects", "/tmp", nil, "")

	// Precondition: #1172 preselected the configured model.
	if got := d.GetLaunchModelID(); got != "claude-opus-4-7" {
		t.Fatalf("precondition: GetLaunchModelID() = %q, want claude-opus-4-7 (#1172 preselect)", got)
	}

	// Move focus to the model field, then pick "Default".
	d.focusIndex = d.indexOf(focusModel)
	d.updateFocus()
	d = selectDefaultEntry(t, d)

	if got := d.GetLaunchModelID(); got != "" {
		t.Fatalf("GetLaunchModelID() = %q after selecting Default, want empty (must override #1172 preselect)", got)
	}
}
