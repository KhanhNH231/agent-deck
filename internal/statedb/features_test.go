package statedb

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFeatureCRUD(t *testing.T) {
	db := newTestDB(t)

	f := FeatureRow{
		ID: "f1", Name: "login", State: "active",
		RootPath: "/ws/login", Conductor: false,
	}
	repos := []FeatureRepoRow{
		{FeatureID: "f1", RepoName: "alpha", RepoPath: "/r/alpha", Branch: "login",
			BaseRef: "origin/main", WorktreePath: "/ws/login/worktrees/alpha"},
		{FeatureID: "f1", RepoName: "beta", RepoPath: "/r/beta", Branch: "login",
			BaseRef: "", WorktreePath: "/ws/login/worktrees/beta"},
	}
	if err := db.SaveFeature(f, repos); err != nil {
		t.Fatalf("SaveFeature: %v", err)
	}

	got, gotRepos, err := db.GetFeatureByName("login")
	if err != nil {
		t.Fatalf("GetFeatureByName: %v", err)
	}
	if got.ID != "f1" || got.State != "active" || got.RootPath != "/ws/login" {
		t.Fatalf("feature = %+v", got)
	}
	if len(gotRepos) != 2 || gotRepos[0].RepoName != "alpha" || gotRepos[1].WorktreePath != "/ws/login/worktrees/beta" {
		t.Fatalf("repos = %+v", gotRepos)
	}

	// Upsert replaces the repo snapshot.
	if err := db.SaveFeature(f, repos[:1]); err != nil {
		t.Fatalf("SaveFeature upsert: %v", err)
	}
	_, gotRepos, _ = db.GetFeatureByName("login")
	if len(gotRepos) != 1 {
		t.Fatalf("after upsert repos = %d, want 1", len(gotRepos))
	}

	if err := db.SetFeatureState("f1", "parked"); err != nil {
		t.Fatalf("SetFeatureState: %v", err)
	}
	got, _, _ = db.GetFeatureByName("login")
	if got.State != "parked" {
		t.Fatalf("state = %q, want parked", got.State)
	}

	all, err := db.ListFeatures()
	if err != nil || len(all) != 1 {
		t.Fatalf("ListFeatures = %v, %v", all, err)
	}

	if err := db.DeleteFeature("f1"); err != nil {
		t.Fatalf("DeleteFeature: %v", err)
	}
	if _, _, err := db.GetFeatureByName("login"); err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestInstanceFeatureIDRoundTrip(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insts := []*InstanceRow{
		{ID: "i1", Title: "T", ProjectPath: "/p", GroupPath: "g", Tool: "claude",
			Status: "idle", CreatedAt: now, FeatureID: "f1", ToolData: json.RawMessage("{}")},
	}
	if err := db.SaveInstances(insts); err != nil {
		t.Fatalf("SaveInstances: %v", err)
	}
	loaded, err := db.LoadInstances()
	if err != nil || len(loaded) != 1 {
		t.Fatalf("LoadInstances: %v len=%d", err, len(loaded))
	}
	if loaded[0].FeatureID != "f1" {
		t.Fatalf("FeatureID = %q, want f1", loaded[0].FeatureID)
	}
}
