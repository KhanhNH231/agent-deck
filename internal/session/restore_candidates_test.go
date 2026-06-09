package session

import (
	"testing"
)

// stubReviver builds a Reviver whose probes are driven by maps keyed on the
// instance's tmux name, so Classify is deterministic without real tmux.
func stubReviver(tmuxAlive, pipeAlive map[string]bool) *Reviver {
	return &Reviver{
		TmuxExists: func(name, _ string) bool { return tmuxAlive[name] },
		PipeAlive:  func(name string) bool { return pipeAlive[name] },
	}
}

// TestCountRestoreCandidates_Predicate covers the trigger predicate: a candidate
// is WasOpen==true AND not currently alive (ClassAlive). WasOpen alive sessions
// and explicitly-closed (!WasOpen) sessions are excluded.
func TestCountRestoreCandidates_Predicate(t *testing.T) {
	// "alive" tmux+pipe both up; "dead" tmux gone; "errored" tmux up pipe down.
	rev := stubReviver(
		map[string]bool{"alive": true, "errored": true, "dead": false, "closed-dead": false},
		map[string]bool{"alive": true, "errored": false},
	)

	insts := []*Instance{
		{ID: "alive", Title: "alive", WasOpen: true, Status: StatusRunning},        // WasOpen + alive → excluded
		{ID: "dead", Title: "dead", WasOpen: true, Status: StatusStopped},          // WasOpen + dead → candidate
		{ID: "errored", Title: "errored", WasOpen: true, Status: StatusRunning},    // WasOpen + errored (not alive) → candidate
		{ID: "closed-dead", Title: "closed-dead", WasOpen: false, Status: StatusStopped}, // !WasOpen → excluded
	}

	got := CountRestoreCandidates(insts, rev)
	if got != 2 {
		t.Fatalf("expected 2 restore candidates, got %d", got)
	}
}

// TestCountRestoreCandidates_NoneWhenAllAliveOrClosed verifies 0 candidates →
// caller will not prompt.
func TestCountRestoreCandidates_NoneWhenAllAliveOrClosed(t *testing.T) {
	rev := stubReviver(
		map[string]bool{"alive": true, "closed": false},
		map[string]bool{"alive": true},
	)
	insts := []*Instance{
		{ID: "alive", Title: "alive", WasOpen: true, Status: StatusRunning},
		{ID: "closed", Title: "closed", WasOpen: false, Status: StatusStopped},
	}
	if got := CountRestoreCandidates(insts, rev); got != 0 {
		t.Fatalf("expected 0 candidates, got %d", got)
	}
}

// TestClassifyRestoreAction maps RevivalClass to the action ladder.
func TestClassifyRestoreAction(t *testing.T) {
	rev := stubReviver(
		map[string]bool{"alive": true, "errored": true, "dead": false},
		map[string]bool{"alive": true, "errored": false},
	)
	cases := []struct {
		name string
		inst *Instance
		want RestoreAction
	}{
		{"alive", &Instance{ID: "alive", Title: "alive", Status: StatusRunning}, RestoreNoop},
		{"errored", &Instance{ID: "errored", Title: "errored", Status: StatusRunning}, RestoreRevive},
		{"dead", &Instance{ID: "dead", Title: "dead", Status: StatusStopped}, RestoreRestart},
		{"nil-deleted", nil, RestoreSkip},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyRestoreAction(tc.inst, rev); got != tc.want {
				t.Errorf("ClassifyRestoreAction = %v, want %v", got, tc.want)
			}
		})
	}
}
