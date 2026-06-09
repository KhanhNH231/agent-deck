package ui

// Tests for the "Default" entry in the in-session Gemini model picker.
//
// "Default" (first list item) means "no model override" — selecting it emits an
// empty model so the gemini CLI uses its own configured default rather than a
// catalog id.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// After models are fetched, "Default" is the first selectable entry.
func TestGeminiDialog_DefaultIsFirstEntry(t *testing.T) {
	d := NewGeminiModelDialog()
	d.SetSize(100, 50)
	d.Show("sess-1", "gemini-2.5-pro")
	d.HandleModelsFetched(modelsFetchedMsg{models: []string{"gemini-2.5-pro", "gemini-2.5-flash"}})

	if len(d.models) == 0 {
		t.Fatal("no models after fetch")
	}
	if d.models[0] != defaultModelSentinel {
		t.Fatalf("models[0] = %q, want %q (Default first)", d.models[0], defaultModelSentinel)
	}
}

// Selecting the "Default" entry emits an empty model (no override).
func TestGeminiDialog_SelectingDefaultEmitsEmptyModel(t *testing.T) {
	d := NewGeminiModelDialog()
	d.SetSize(100, 50)
	d.Show("sess-1", "gemini-2.5-pro")
	d.HandleModelsFetched(modelsFetchedMsg{models: []string{"gemini-2.5-pro", "gemini-2.5-flash"}})

	// Move cursor to the Default entry (index 0) and select it.
	d.cursor = 0
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on Default produced no command")
	}
	msg := cmd()
	selected, ok := msg.(modelSelectedMsg)
	if !ok {
		t.Fatalf("Enter produced %T, want modelSelectedMsg", msg)
	}
	if selected.model != "" {
		t.Fatalf("selected.model = %q, want empty (Default => no override)", selected.model)
	}
	if selected.instanceID != "sess-1" {
		t.Fatalf("selected.instanceID = %q, want sess-1", selected.instanceID)
	}
}

// Selecting a real catalog model still emits that exact id (Default must not
// swallow normal selection).
func TestGeminiDialog_SelectingRealModelStillWorks(t *testing.T) {
	d := NewGeminiModelDialog()
	d.SetSize(100, 50)
	d.Show("sess-1", "gemini-2.5-pro")
	d.HandleModelsFetched(modelsFetchedMsg{models: []string{"gemini-2.5-pro", "gemini-2.5-flash"}})

	// models == ["Default", "gemini-2.5-pro", "gemini-2.5-flash"]; pick the last.
	d.cursor = len(d.models) - 1
	want := d.models[d.cursor]
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a real model produced no command")
	}
	selected, ok := cmd().(modelSelectedMsg)
	if !ok {
		t.Fatalf("Enter produced wrong message type")
	}
	if selected.model != want {
		t.Fatalf("selected.model = %q, want %q", selected.model, want)
	}
}
