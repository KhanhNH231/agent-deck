package session

import (
	"testing"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/statedb"
)

// TestWasOpen_InstanceToRow verifies instanceToRow carries WasOpen into the
// statedb row (the save direction).
func TestWasOpen_InstanceToRow(t *testing.T) {
	for _, want := range []bool{true, false} {
		inst := &Instance{
			ID:          "i1",
			Title:       "T",
			ProjectPath: "/tmp",
			GroupPath:   "grp",
			Tool:        "claude",
			Status:      StatusIdle,
			CreatedAt:   time.Now(),
			WasOpen:     want,
		}
		row, err := instanceToRow(inst)
		if err != nil {
			t.Fatalf("instanceToRow: %v", err)
		}
		if row.WasOpen != want {
			t.Errorf("instanceToRow WasOpen = %v, want %v", row.WasOpen, want)
		}
	}
}

// TestWasOpen_StorageRoundTrip verifies WasOpen survives a full
// SaveWithGroups → LoadWithGroups cycle (the load direction goes through
// InstanceData → Instance, mirroring TitleLocked).
func TestWasOpen_StorageRoundTrip(t *testing.T) {
	s := newTestStorage(t)

	instances := []*Instance{
		{ID: "open", Title: "Open", ProjectPath: "/tmp/a", GroupPath: "grp", Command: "claude", Tool: "claude", Status: StatusIdle, CreatedAt: time.Now(), WasOpen: true},
		{ID: "closed", Title: "Closed", ProjectPath: "/tmp/b", GroupPath: "grp", Command: "claude", Tool: "claude", Status: StatusIdle, CreatedAt: time.Now(), WasOpen: false},
	}
	if err := s.SaveWithGroups(instances, nil); err != nil {
		t.Fatalf("SaveWithGroups: %v", err)
	}

	loaded, _, err := s.LoadWithGroups()
	if err != nil {
		t.Fatalf("LoadWithGroups: %v", err)
	}
	byID := map[string]*Instance{}
	for _, inst := range loaded {
		byID[inst.ID] = inst
	}
	if got := byID["open"]; got == nil || !got.WasOpen {
		t.Errorf("expected open.WasOpen=true, got %+v", got)
	}
	if got := byID["closed"]; got == nil || got.WasOpen {
		t.Errorf("expected closed.WasOpen=false, got %+v", got)
	}
}

// guard: assert the statedb.InstanceRow has the field this package depends on.
var _ = statedb.InstanceRow{WasOpen: true}
