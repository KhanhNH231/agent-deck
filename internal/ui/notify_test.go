package ui

import (
	"testing"
	"time"

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

// --- notifyTracker dwell/debounce behavior (false-positive flicker fix) ---

const testDwell = 5 * time.Second

func newTestTime() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

// A status that is the very first thing the tracker sees for an instance is the
// baseline (prev == ""), never an observed transition — so it must not fire,
// even if it is Waiting and held well past the dwell. Prevents a ping storm for
// already-waiting sessions at app startup.
func TestNotifyTracker_BaselineNeverFires(t *testing.T) {
	tr := newNotifyTracker()
	t0 := newTestTime()
	if fire, _ := tr.observe("a", session.StatusWaiting, false, allOn(), t0, testDwell); fire {
		t.Fatal("baseline observe should not fire")
	}
	if fire, _ := tr.observe("a", session.StatusWaiting, false, allOn(), t0.Add(10*time.Second), testDwell); fire {
		t.Fatal("baseline that stays Waiting past dwell should still not fire")
	}
}

// A genuine transition that holds past the dwell fires exactly once.
func TestNotifyTracker_FiresOnceAfterDwell(t *testing.T) {
	tr := newNotifyTracker()
	t0 := newTestTime()
	// baseline running
	tr.observe("a", session.StatusRunning, false, allOn(), t0, testDwell)
	// transition into waiting; dwell not yet elapsed
	if fire, _ := tr.observe("a", session.StatusWaiting, false, allOn(), t0.Add(1*time.Second), testDwell); fire {
		t.Fatal("should not fire before dwell elapses")
	}
	// dwell elapsed → fires needs-input
	fire, event := tr.observe("a", session.StatusWaiting, false, allOn(), t0.Add(1*time.Second).Add(testDwell), testDwell)
	if !fire || event != notifyEventNeedsInput {
		t.Fatalf("expected needs-input fire, got fire=%v event=%q", fire, event)
	}
	// same stable episode → does not re-fire
	if fire, _ := tr.observe("a", session.StatusWaiting, false, allOn(), t0.Add(20*time.Second), testDwell); fire {
		t.Fatal("should not re-fire within the same episode")
	}
}

// Rapid flips shorter than the dwell never fire; once the status settles for
// >= dwell it fires.
func TestNotifyTracker_FlickerSuppressedThenFires(t *testing.T) {
	tr := newNotifyTracker()
	t0 := newTestTime()
	tr.observe("a", session.StatusRunning, false, allOn(), t0, testDwell)
	// oscillate every 2s (< 5s dwell): none of these episodes survives the dwell
	flips := []struct {
		s  session.Status
		dt time.Duration
	}{
		{session.StatusWaiting, 2 * time.Second},
		{session.StatusRunning, 4 * time.Second},
		{session.StatusWaiting, 6 * time.Second},
		{session.StatusError, 8 * time.Second},
		{session.StatusRunning, 10 * time.Second},
	}
	for _, f := range flips {
		if fire, _ := tr.observe("a", f.s, false, allOn(), t0.Add(f.dt), testDwell); fire {
			t.Fatalf("flicker into %s should not fire", f.s)
		}
	}
	// now settle into waiting and hold past the dwell
	tr.observe("a", session.StatusWaiting, false, allOn(), t0.Add(12*time.Second), testDwell)
	fire, event := tr.observe("a", session.StatusWaiting, false, allOn(), t0.Add(12*time.Second).Add(testDwell), testDwell)
	if !fire || event != notifyEventNeedsInput {
		t.Fatalf("settled waiting should fire needs-input, got fire=%v event=%q", fire, event)
	}
}

// Attached pane is noise — never fires even after the dwell.
func TestNotifyTracker_AttachedSuppressed(t *testing.T) {
	tr := newNotifyTracker()
	t0 := newTestTime()
	tr.observe("a", session.StatusRunning, true, allOn(), t0, testDwell)
	tr.observe("a", session.StatusWaiting, true, allOn(), t0.Add(1*time.Second), testDwell)
	if fire, _ := tr.observe("a", session.StatusWaiting, true, allOn(), t0.Add(testDwell).Add(2*time.Second), testDwell); fire {
		t.Fatal("attached pane should not fire")
	}
}

// Running->Idle held past the dwell is a "finished" event; Idle reached from
// Waiting (acknowledged) is not.
func TestNotifyTracker_FinishedVsAcknowledged(t *testing.T) {
	// finished: baseline running -> idle
	tr := newNotifyTracker()
	t0 := newTestTime()
	tr.observe("fin", session.StatusRunning, false, allOn(), t0, testDwell)
	tr.observe("fin", session.StatusIdle, false, allOn(), t0.Add(1*time.Second), testDwell)
	fire, event := tr.observe("fin", session.StatusIdle, false, allOn(), t0.Add(1*time.Second).Add(testDwell), testDwell)
	if !fire || event != notifyEventFinished {
		t.Fatalf("running->idle should fire finished, got fire=%v event=%q", fire, event)
	}

	// acknowledged: baseline waiting -> idle must NOT fire finished
	tr2 := newNotifyTracker()
	tr2.observe("ack", session.StatusWaiting, false, allOn(), t0, testDwell)
	tr2.observe("ack", session.StatusIdle, false, allOn(), t0.Add(1*time.Second), testDwell)
	if fire, _ := tr2.observe("ack", session.StatusIdle, false, allOn(), t0.Add(1*time.Second).Add(testDwell), testDwell); fire {
		t.Fatal("waiting->idle (acknowledged) should not fire finished")
	}
}

// prune drops episodes for instances no longer present.
func TestNotifyTracker_Prune(t *testing.T) {
	tr := newNotifyTracker()
	t0 := newTestTime()
	tr.observe("keep", session.StatusRunning, false, allOn(), t0, testDwell)
	tr.observe("drop", session.StatusRunning, false, allOn(), t0, testDwell)
	tr.prune(map[string]bool{"keep": true})
	if _, ok := tr.episodes["drop"]; ok {
		t.Fatal("prune should have removed 'drop'")
	}
	if _, ok := tr.episodes["keep"]; !ok {
		t.Fatal("prune should have kept 'keep'")
	}
}
