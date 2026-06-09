package session

// Tests for the "Default" model-picker option semantics on the session side.
//
// "Default" means "no per-session model override" — the agent CLI uses whatever
// it is configured with, and ToArgs() emits no --model/-m flag. The picker
// represents this as an empty model string flowing into ApplyLaunchModel("").
//
// Distinct from the [claude].default_model config (#1172), which force-selects a
// specific catalog id. "Default" is the explicit "no override" choice and must
// clear any model previously stored on the session.

import (
	"slices"
	"testing"
)

// ApplyLaunchModel("") must clear a model that was set earlier, for every tool
// SupportsLaunchModel covers. Otherwise selecting "Default" after picking a real
// model would leave a stale --model flag on the next launch.
func TestApplyLaunchModel_EmptyClearsPriorOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	ClearUserConfigCache()
	t.Cleanup(ClearUserConfigCache)

	tools := []string{"claude", "gemini", "opencode", "codex"}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			inst := NewInstanceWithTool("default-"+tool, "/tmp/model", tool)

			// Establish a real override first.
			if err := inst.ApplyLaunchModel("claude-sonnet-4-6"); err != nil {
				t.Fatalf("ApplyLaunchModel(real): %v", err)
			}
			if got := inst.LaunchModelID(); got == "" {
				t.Fatalf("precondition: LaunchModelID() empty after setting a real model")
			}

			// Selecting "Default" => empty => must clear the override.
			if err := inst.ApplyLaunchModel(""); err != nil {
				t.Fatalf("ApplyLaunchModel(\"\"): %v", err)
			}
			if got := inst.LaunchModelID(); got != "" {
				t.Fatalf("LaunchModelID() = %q after ApplyLaunchModel(\"\"), want empty (stale override left)", got)
			}
		})
	}
}

// After selecting "Default", no tool's ToArgs may emit a --model/-m flag.
func TestApplyLaunchModel_EmptyEmitsNoModelFlag(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	ClearUserConfigCache()
	t.Cleanup(ClearUserConfigCache)

	assertNoModelFlag := func(t *testing.T, args []string) {
		t.Helper()
		if slices.Contains(args, "--model") || slices.Contains(args, "-m") {
			t.Fatalf("ToArgs emitted a model flag after Default selected: %v", args)
		}
	}

	// claude
	claude := NewInstanceWithTool("default-claude", "/tmp/model", "claude")
	if err := claude.ApplyLaunchModel("claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	if err := claude.ApplyLaunchModel(""); err != nil {
		t.Fatal(err)
	}
	assertNoModelFlag(t, claude.GetClaudeOptions().ToArgs())

	// codex
	codex := NewInstanceWithTool("default-codex", "/tmp/model", "codex")
	if err := codex.ApplyLaunchModel("gpt-5.5"); err != nil {
		t.Fatal(err)
	}
	if err := codex.ApplyLaunchModel(""); err != nil {
		t.Fatal(err)
	}
	assertNoModelFlag(t, codex.GetCodexOptions().ToArgs())

	// opencode
	opencode := NewInstanceWithTool("default-opencode", "/tmp/model", "opencode")
	if err := opencode.ApplyLaunchModel("openai/gpt-5.5"); err != nil {
		t.Fatal(err)
	}
	if err := opencode.ApplyLaunchModel(""); err != nil {
		t.Fatal(err)
	}
	assertNoModelFlag(t, opencode.GetOpenCodeOptions().ToArgs())
}
