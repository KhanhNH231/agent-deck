package statedb

import (
	"encoding/json"
	"testing"
	"time"
)

// TestWasOpen_RoundTrip verifies the was_open boolean persists and reloads
// through SaveInstances/LoadInstances (the bulk path) for both true and false.
func TestWasOpen_RoundTrip(t *testing.T) {
	db := newTestDB(t)

	now := time.Now()
	instances := []*InstanceRow{
		{ID: "open", Title: "Open", ProjectPath: "/a", GroupPath: "grp", Order: 0, Tool: "claude", Status: "idle", CreatedAt: now, WasOpen: true, ToolData: json.RawMessage("{}")},
		{ID: "closed", Title: "Closed", ProjectPath: "/b", GroupPath: "grp", Order: 1, Tool: "claude", Status: "idle", CreatedAt: now, WasOpen: false, ToolData: json.RawMessage("{}")},
	}

	if err := db.SaveInstances(instances); err != nil {
		t.Fatalf("SaveInstances: %v", err)
	}

	loaded, err := db.LoadInstances()
	if err != nil {
		t.Fatalf("LoadInstances: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(loaded))
	}
	byID := map[string]*InstanceRow{}
	for _, r := range loaded {
		byID[r.ID] = r
	}
	if !byID["open"].WasOpen {
		t.Errorf("expected open.WasOpen=true, got false")
	}
	if byID["closed"].WasOpen {
		t.Errorf("expected closed.WasOpen=false, got true")
	}
}

// TestWasOpen_SaveInstanceSingleRow verifies the single-row SaveInstance path
// (used by InsertSessionAndVerify) also persists was_open.
func TestWasOpen_SaveInstanceSingleRow(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveInstance(&InstanceRow{
		ID: "single", Title: "Single", ProjectPath: "/tmp", GroupPath: "grp",
		Tool: "claude", Status: "idle", CreatedAt: time.Now(), WasOpen: true,
		ToolData: json.RawMessage("{}"),
	}); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}

	loaded, err := db.LoadInstances()
	if err != nil {
		t.Fatalf("LoadInstances: %v", err)
	}
	if len(loaded) != 1 || !loaded[0].WasOpen {
		t.Fatalf("expected 1 instance with WasOpen=true, got %+v", loaded)
	}
}

// TestMigrate_OldSchema_WasOpenColumn verifies that upgrading from the v1
// schema (which has no was_open column) adds the column and that legacy rows
// load as WasOpen=false (the DEFAULT 0 contract).
func TestMigrate_OldSchema_WasOpenColumn(t *testing.T) {
	db := createV1SchemaDB(t)

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() on v1 schema failed: %v", err)
	}

	instances, err := db.LoadInstances()
	if err != nil {
		t.Fatalf("LoadInstances after migrate: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance after migrate, got %d", len(instances))
	}
	// Legacy row predates was_open → must load false.
	if instances[0].WasOpen {
		t.Errorf("expected legacy row WasOpen=false, got true")
	}

	// And a fresh write on the migrated DB can set it true.
	if err := db.SaveInstance(&InstanceRow{
		ID: "post-migrate", Title: "Post", ProjectPath: "/tmp", GroupPath: "grp",
		Tool: "claude", Status: "idle", CreatedAt: time.Now(), WasOpen: true,
		ToolData: json.RawMessage("{}"),
	}); err != nil {
		t.Fatalf("SaveInstance after migrate: %v", err)
	}
	instances, _ = db.LoadInstances()
	var found bool
	for _, r := range instances {
		if r.ID == "post-migrate" {
			found = true
			if !r.WasOpen {
				t.Errorf("expected post-migrate WasOpen=true, got false")
			}
		}
	}
	if !found {
		t.Errorf("post-migrate row not found after migrate")
	}
}

// TestSchemaVersion_BumpedTo10 documents the intentional bump that accompanies
// the was_open column. If this fails, the migration block needs a matching
// oldVer guard.
func TestSchemaVersion_BumpedTo10(t *testing.T) {
	if SchemaVersion < 10 {
		t.Fatalf("expected SchemaVersion >= 10 after was_open column, got %d", SchemaVersion)
	}
}
