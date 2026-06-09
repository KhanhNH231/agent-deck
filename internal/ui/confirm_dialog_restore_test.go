package ui

import (
	"strings"
	"testing"
)

// TestShowRestoreSessions_State verifies ShowRestoreSessions wires the dialog
// type, target group path, count, button count, and the default focused button
// (the safe DECLINE button, index 1).
func TestShowRestoreSessions_State(t *testing.T) {
	c := NewConfirmDialog()
	c.ShowRestoreSessions("projects/devops", 3)

	if !c.IsVisible() {
		t.Fatalf("expected dialog visible after ShowRestoreSessions")
	}
	if c.GetConfirmType() != ConfirmRestoreSessions {
		t.Fatalf("expected ConfirmRestoreSessions, got %v", c.GetConfirmType())
	}
	if c.GetTargetID() != "projects/devops" {
		t.Fatalf("expected targetID=groupPath, got %q", c.GetTargetID())
	}
	if c.mcpCount != 3 {
		t.Fatalf("expected count 3 carried, got %d", c.mcpCount)
	}
	if c.buttonCount != 2 {
		t.Fatalf("expected buttonCount=2, got %d", c.buttonCount)
	}
	if c.GetFocusedButton() != 1 {
		t.Fatalf("expected default focus on DECLINE button (1), got %d", c.GetFocusedButton())
	}
}

// TestRestoreSessions_View renders the count and the group name.
func TestRestoreSessions_View(t *testing.T) {
	c := NewConfirmDialog()
	c.SetSize(100, 40)
	c.ShowRestoreSessions("projects/devops", 3)

	out := c.View()
	if !strings.Contains(out, "Restore 3 previous sessions?") {
		t.Errorf("View missing title with count, got:\n%s", out)
	}
	if !strings.Contains(out, "projects/devops") {
		t.Errorf("View missing group name, got:\n%s", out)
	}
	if !strings.Contains(out, "Restore") || !strings.Contains(out, "Not now") {
		t.Errorf("View missing Restore / Not now buttons, got:\n%s", out)
	}
}
