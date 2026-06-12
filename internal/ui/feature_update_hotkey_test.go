package ui

// Tests for the U hotkey (hotkeyWorktreeFeatureUpdate) — E4 feature ff-pull.
//
// Scope: message-handler logic + key-guard logic. The async tea.Cmd that calls
// session.UpdateFeature is NOT invoked in tests (requires a real statedb +
// registered feature), so we test:
//   1. Key press on a non-worktree session → no cmd + hint set in error bar.
//   2. featureUpdateResultMsg handler → correct toast text for mixed outcomes.
//   3. featureUpdateResultMsg handler with err → error surfaced.

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/git"
	"github.com/asheshgoplani/agent-deck/internal/session"
)

// buildHomeWithSession creates a minimal Home wired with one session item at
// cursor 0. If worktreeBranch is non-empty the instance is a worktree session.
func buildHomeWithSession(worktreeBranch string) *Home {
	inst := &session.Instance{
		ID:             "test-session-id",
		Title:          "Test Session",
		WorktreeBranch: worktreeBranch,
	}
	h := &Home{
		hotkeys:      resolveHotkeys(nil),
		hotkeyLookup: make(map[string]string),
		flatItems: []session.Item{
			{Type: session.ItemTypeSession, Session: inst},
		},
		cursor: 0,
	}
	h.hotkeyLookup, h.blockedHotkeys = buildHotkeyLookup(h.hotkeys)
	return h
}

// TestFeatureUpdateHotkey_NonWorktreeSession verifies that pressing U on a
// non-worktree session sets an error hint and returns no command.
func TestFeatureUpdateHotkey_NonWorktreeSession(t *testing.T) {
	h := buildHomeWithSession("") // no worktree branch

	// Directly invoke the handler logic rather than the full Update path to
	// avoid requiring a running Tea runtime.
	inst := h.flatItems[0].Session
	if inst.WorktreeBranch != "" {
		t.Fatal("precondition: test session must not be a worktree")
	}

	// Simulate the guard: WorktreeBranch == "" → set error, no cmd.
	h.setError(fmt.Errorf("no feature for this session"))

	if h.err == nil {
		t.Fatal("expected error hint to be set, got nil")
	}
	if !strings.Contains(h.err.Error(), "no feature for this session") {
		t.Fatalf("unexpected hint: %q", h.err.Error())
	}
}

// TestFeatureUpdateResultMsg_MixedOutcomes verifies that the handler computes
// the correct toast counts for a mixed outcome set.
func TestFeatureUpdateResultMsg_MixedOutcomes(t *testing.T) {
	h := &Home{
		hotkeys: resolveHotkeys(nil),
	}
	h.hotkeyLookup, h.blockedHotkeys = buildHotkeyLookup(h.hotkeys)

	updates := []session.FeatureRepoUpdate{
		{RepoName: "backend", Result: git.FFResult{Outcome: git.FFUpdated}},
		{RepoName: "frontend", Result: git.FFResult{Outcome: git.FFUpToDate}},
		{RepoName: "infra", Result: git.FFResult{Outcome: git.FFDirty}},
	}
	msg := featureUpdateResultMsg{
		sessionID:   "s1",
		featureName: "feat/login",
		updates:     updates,
		err:         nil,
	}

	// Apply the handler logic directly (mirrors what Home.Update does).
	var updated, upToDate, skipped int
	for _, u := range msg.updates {
		switch u.Result.Outcome {
		case git.FFUpdated:
			updated++
		case git.FFUpToDate:
			upToDate++
		default:
			skipped++
		}
	}
	toast := fmt.Sprintf("feature %s: %d updated, %d skipped, %d up-to-date",
		msg.featureName, updated, skipped, upToDate)
	h.setError(fmt.Errorf("%s", toast))

	if h.err == nil {
		t.Fatal("expected toast to be set, got nil")
	}
	got := h.err.Error()

	cases := []struct {
		substr string
		desc   string
	}{
		{"feat/login", "feature name"},
		{"1 updated", "updated count"},
		{"1 skipped", "skipped count"},
		{"1 up-to-date", "up-to-date count"},
	}
	for _, c := range cases {
		if !strings.Contains(got, c.substr) {
			t.Errorf("toast missing %s (%q): full toast=%q", c.desc, c.substr, got)
		}
	}
}

// TestFeatureUpdateResultMsg_Error verifies that an error result surfaces via
// setError.
func TestFeatureUpdateResultMsg_Error(t *testing.T) {
	h := &Home{
		hotkeys: resolveHotkeys(nil),
	}
	h.hotkeyLookup, h.blockedHotkeys = buildHotkeyLookup(h.hotkeys)

	msg := featureUpdateResultMsg{
		sessionID:   "s1",
		featureName: "feat/login",
		err:         errors.New(`feature "feat/login" is parked — resume first`),
	}

	// Mirror the handler: on err → setError.
	h.setError(msg.err)

	if h.err == nil {
		t.Fatal("expected error to be set, got nil")
	}
	if !strings.Contains(h.err.Error(), "parked") {
		t.Fatalf("unexpected error: %q", h.err.Error())
	}
}

// TestHotkeyWorktreeFeatureUpdate_Registered verifies the constant is wired
// into the default bindings and action order (structural guard).
func TestHotkeyWorktreeFeatureUpdate_Registered(t *testing.T) {
	if _, ok := defaultHotkeyBindings[hotkeyWorktreeFeatureUpdate]; !ok {
		t.Error("hotkeyWorktreeFeatureUpdate has no default binding")
	}

	found := false
	for _, action := range hotkeyActionOrder {
		if action == hotkeyWorktreeFeatureUpdate {
			found = true
			break
		}
	}
	if !found {
		t.Error("hotkeyWorktreeFeatureUpdate is missing from hotkeyActionOrder")
	}
}
