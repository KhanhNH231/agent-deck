package ui

import (
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// reviverAllDead returns a Reviver whose probes report every session dead
// (tmux gone), so a WasOpen session is always a restore candidate.
func reviverAllDead() *session.Reviver {
	return &session.Reviver{
		TmuxExists: func(_, _ string) bool { return false },
		PipeAlive:  func(_ string) bool { return false },
	}
}

func newRestoreTestHome() *Home {
	return &Home{
		confirmDialog:   NewConfirmDialog(),
		restorePrompted: map[string]bool{},
		restoreReviver:  reviverAllDead(),
		instanceByID:    map[string]*session.Instance{},
	}
}

// TestMaybePromptRestore_ShowsWhenCandidates verifies a top-level group with
// WasOpen-but-dead sessions triggers the prompt with the right count.
func TestMaybePromptRestore_ShowsWhenCandidates(t *testing.T) {
	h := newRestoreTestHome()
	h.instances = []*session.Instance{
		{ID: "a", Title: "A", GroupPath: "proj", WasOpen: true, Status: session.StatusStopped},
		{ID: "b", Title: "B", GroupPath: "proj", WasOpen: true, Status: session.StatusStopped},
		{ID: "c", Title: "C", GroupPath: "proj", WasOpen: false, Status: session.StatusStopped}, // explicitly closed → excluded
	}

	h.maybePromptRestore("proj")

	if !h.confirmDialog.IsVisible() || h.confirmDialog.GetConfirmType() != ConfirmRestoreSessions {
		t.Fatalf("expected restore prompt visible, got visible=%v type=%v",
			h.confirmDialog.IsVisible(), h.confirmDialog.GetConfirmType())
	}
	if h.confirmDialog.mcpCount != 2 {
		t.Fatalf("expected count 2, got %d", h.confirmDialog.mcpCount)
	}
}

// TestMaybePromptRestore_NoCandidates verifies no prompt when nothing qualifies.
func TestMaybePromptRestore_NoCandidates(t *testing.T) {
	h := newRestoreTestHome()
	h.instances = []*session.Instance{
		{ID: "c", Title: "C", GroupPath: "proj", WasOpen: false, Status: session.StatusStopped},
	}
	h.maybePromptRestore("proj")
	if h.confirmDialog.IsVisible() {
		t.Fatalf("expected no prompt for zero candidates")
	}
}

// TestMaybePromptRestore_TopLevelOnly verifies a nested (Level>0) group never
// prompts.
func TestMaybePromptRestore_TopLevelOnly(t *testing.T) {
	h := newRestoreTestHome()
	h.instances = []*session.Instance{
		{ID: "a", Title: "A", GroupPath: "proj/sub", WasOpen: true, Status: session.StatusStopped},
	}
	h.maybePromptRestore("proj/sub")
	if h.confirmDialog.IsVisible() {
		t.Fatalf("expected no prompt for nested group")
	}
}

// TestMaybePromptRestore_DedupSameSession verifies a second expand of the same
// group does not re-prompt within the app session.
func TestMaybePromptRestore_DedupSameSession(t *testing.T) {
	h := newRestoreTestHome()
	h.instances = []*session.Instance{
		{ID: "a", Title: "A", GroupPath: "proj", WasOpen: true, Status: session.StatusStopped},
	}

	h.maybePromptRestore("proj")
	if !h.confirmDialog.IsVisible() {
		t.Fatalf("expected first expand to prompt")
	}
	// Dismiss (as if user answered) and expand again.
	h.confirmDialog.Hide()
	h.maybePromptRestore("proj")
	if h.confirmDialog.IsVisible() {
		t.Fatalf("expected second expand NOT to re-prompt (dedup)")
	}
}
