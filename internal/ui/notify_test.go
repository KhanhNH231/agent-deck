package ui

import (
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// allOn is the fully-enabled NotifyConfig used by most cases so each test
// isolates exactly one variable.
func allOn() NotifyConfig {
	return NotifyConfig{
		ITermEnabled: true,
		OnNeedsInput: true,
		OnFinished:   true,
		OnError:      true,
	}
}

func TestNotifyDecision(t *testing.T) {
	tests := []struct {
		name       string
		old        session.Status
		new        session.Status
		isAttached bool
		cfg        NotifyConfig
		wantFire   bool
		wantEvent  string
	}{
		// --- the three qualifying transitions, background + enabled ---
		{
			name:     "running->waiting fires needs input",
			old:      session.StatusRunning,
			new:      session.StatusWaiting,
			cfg:      allOn(),
			wantFire: true, wantEvent: "needs input",
		},
		{
			name:     "idle->waiting fires needs input (any non-waiting old)",
			old:      session.StatusIdle,
			new:      session.StatusWaiting,
			cfg:      allOn(),
			wantFire: true, wantEvent: "needs input",
		},
		{
			name:     "running->idle fires finished",
			old:      session.StatusRunning,
			new:      session.StatusIdle,
			cfg:      allOn(),
			wantFire: true, wantEvent: "finished",
		},
		{
			name:     "running->error fires error",
			old:      session.StatusRunning,
			new:      session.StatusError,
			cfg:      allOn(),
			wantFire: true, wantEvent: "error",
		},
		{
			name:     "idle->error fires error (any non-error old)",
			old:      session.StatusIdle,
			new:      session.StatusError,
			cfg:      allOn(),
			wantFire: true, wantEvent: "error",
		},

		// --- attached => never fire (it's the pane you're staring at) ---
		{
			name:       "running->waiting suppressed when attached",
			old:        session.StatusRunning,
			new:        session.StatusWaiting,
			isAttached: true,
			cfg:        allOn(),
			wantFire:   false,
		},
		{
			name:       "running->idle suppressed when attached",
			old:        session.StatusRunning,
			new:        session.StatusIdle,
			isAttached: true,
			cfg:        allOn(),
			wantFire:   false,
		},
		{
			name:       "running->error suppressed when attached",
			old:        session.StatusRunning,
			new:        session.StatusError,
			isAttached: true,
			cfg:        allOn(),
			wantFire:   false,
		},

		// --- non-qualifying transitions never fire ---
		{
			name:     "starting->running does not fire",
			old:      session.StatusStarting,
			new:      session.StatusRunning,
			cfg:      allOn(),
			wantFire: false,
		},
		{
			name:     "queued->running does not fire",
			old:      session.StatusQueued,
			new:      session.StatusRunning,
			cfg:      allOn(),
			wantFire: false,
		},
		{
			name:     "idle->running does not fire",
			old:      session.StatusIdle,
			new:      session.StatusRunning,
			cfg:      allOn(),
			wantFire: false,
		},
		{
			name:     "waiting->idle does not fire needs-input nor finished",
			old:      session.StatusWaiting,
			new:      session.StatusIdle,
			cfg:      allOn(),
			wantFire: false,
		},
		{
			name:     "waiting->waiting does not fire (not a transition)",
			old:      session.StatusWaiting,
			new:      session.StatusWaiting,
			cfg:      allOn(),
			wantFire: false,
		},
		{
			name:     "error->error does not fire (already errored)",
			old:      session.StatusError,
			new:      session.StatusError,
			cfg:      allOn(),
			wantFire: false,
		},
		{
			name:     "starting->idle does not fire finished (only running->idle)",
			old:      session.StatusStarting,
			new:      session.StatusIdle,
			cfg:      allOn(),
			wantFire: false,
		},
		{
			name:     "stopped->idle does not fire finished (only running->idle)",
			old:      session.StatusStopped,
			new:      session.StatusIdle,
			cfg:      allOn(),
			wantFire: false,
		},

		// --- master switch off => suppress everything ---
		{
			name: "iterm_enabled=false suppresses needs input",
			old:  session.StatusRunning, new: session.StatusWaiting,
			cfg:      NotifyConfig{ITermEnabled: false, OnNeedsInput: true, OnFinished: true, OnError: true},
			wantFire: false,
		},
		{
			name: "iterm_enabled=false suppresses finished",
			old:  session.StatusRunning, new: session.StatusIdle,
			cfg:      NotifyConfig{ITermEnabled: false, OnNeedsInput: true, OnFinished: true, OnError: true},
			wantFire: false,
		},
		{
			name: "iterm_enabled=false suppresses error",
			old:  session.StatusRunning, new: session.StatusError,
			cfg:      NotifyConfig{ITermEnabled: false, OnNeedsInput: true, OnFinished: true, OnError: true},
			wantFire: false,
		},

		// --- per-event flags suppress only their own event ---
		{
			name: "on_needs_input=false suppresses only needs input",
			old:  session.StatusRunning, new: session.StatusWaiting,
			cfg:      NotifyConfig{ITermEnabled: true, OnNeedsInput: false, OnFinished: true, OnError: true},
			wantFire: false,
		},
		{
			name: "on_needs_input=false leaves finished firing",
			old:  session.StatusRunning, new: session.StatusIdle,
			cfg:      NotifyConfig{ITermEnabled: true, OnNeedsInput: false, OnFinished: true, OnError: true},
			wantFire: true, wantEvent: "finished",
		},
		{
			name: "on_finished=false suppresses only finished",
			old:  session.StatusRunning, new: session.StatusIdle,
			cfg:      NotifyConfig{ITermEnabled: true, OnNeedsInput: true, OnFinished: false, OnError: true},
			wantFire: false,
		},
		{
			name: "on_finished=false leaves error firing",
			old:  session.StatusRunning, new: session.StatusError,
			cfg:      NotifyConfig{ITermEnabled: true, OnNeedsInput: true, OnFinished: false, OnError: true},
			wantFire: true, wantEvent: "error",
		},
		{
			name: "on_error=false suppresses only error",
			old:  session.StatusRunning, new: session.StatusError,
			cfg:      NotifyConfig{ITermEnabled: true, OnNeedsInput: true, OnFinished: true, OnError: false},
			wantFire: false,
		},
		{
			name: "on_error=false leaves needs input firing",
			old:  session.StatusRunning, new: session.StatusWaiting,
			cfg:      NotifyConfig{ITermEnabled: true, OnNeedsInput: true, OnFinished: true, OnError: false},
			wantFire: true, wantEvent: "needs input",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fire, event := notifyDecision(tc.old, tc.new, tc.isAttached, tc.cfg)
			if fire != tc.wantFire {
				t.Fatalf("fire: got %v want %v (event=%q)", fire, tc.wantFire, event)
			}
			if fire && event != tc.wantEvent {
				t.Fatalf("event: got %q want %q", event, tc.wantEvent)
			}
		})
	}
}
