package session

import (
	"testing"
	"time"
)

// TestWorkspaceAutoUpdateDefaults verifies that absent auto_update fields
// produce the expected defaults: disabled, 30m interval.
func TestWorkspaceAutoUpdateDefaults(t *testing.T) {
	writeWorkspaceConfig(t, `
[workspace]
root = "~/ws"
`)
	ws := GetWorkspaceSettings()
	if ws.AutoUpdateEnabled() {
		t.Error("AutoUpdateEnabled: want false (disabled by default), got true")
	}
	if got, want := ws.AutoUpdateInterval(), 30*time.Minute; got != want {
		t.Errorf("AutoUpdateInterval: want %v, got %v", want, got)
	}
}

// TestWorkspaceAutoUpdateEnabled verifies that explicitly enabling auto_update
// and setting a custom interval is parsed correctly.
func TestWorkspaceAutoUpdateEnabled(t *testing.T) {
	writeWorkspaceConfig(t, `
[workspace]
root = "~/ws"
auto_update = true
auto_update_interval_minutes = 15
`)
	ws := GetWorkspaceSettings()
	if !ws.AutoUpdateEnabled() {
		t.Error("AutoUpdateEnabled: want true, got false")
	}
	if got, want := ws.AutoUpdateInterval(), 15*time.Minute; got != want {
		t.Errorf("AutoUpdateInterval: want %v, got %v", want, got)
	}
}

// TestWorkspaceAutoUpdateIntervalClamp verifies that a value below the 5-minute
// floor is clamped up to 5 minutes.
func TestWorkspaceAutoUpdateIntervalClamp(t *testing.T) {
	writeWorkspaceConfig(t, `
[workspace]
root = "~/ws"
auto_update = true
auto_update_interval_minutes = 2
`)
	ws := GetWorkspaceSettings()
	if got, want := ws.AutoUpdateInterval(), 5*time.Minute; got != want {
		t.Errorf("AutoUpdateInterval (clamp): want %v, got %v", want, got)
	}
}
