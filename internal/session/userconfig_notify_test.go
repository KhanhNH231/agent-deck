package session

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestNotificationsConfig_ITermDefaults pins the default-true semantics of the
// new iTerm2 notification flags: an empty [notifications] section (no keys
// written) must report iterm_enabled / on_needs_input / on_finished / on_error
// all true. Defaults live in the getters (nil pointer => true) so a fresh
// install gets notifications without writing any config.
func TestNotificationsConfig_ITermDefaults(t *testing.T) {
	var cfg NotificationsConfig // zero value: all pointers nil

	if !cfg.GetITermEnabled() {
		t.Error("GetITermEnabled must default to true when unset")
	}
	if !cfg.GetOnNeedsInput() {
		t.Error("GetOnNeedsInput must default to true when unset")
	}
	if !cfg.GetOnFinished() {
		t.Error("GetOnFinished must default to true when unset")
	}
	if !cfg.GetOnError() {
		t.Error("GetOnError must default to true when unset")
	}
}

// TestNotificationsConfig_ITermExplicitFalseRoundTrip verifies the flags
// round-trip through TOML and that an explicit false is honored (distinct from
// unset). This is the path a user takes to opt out of a specific event.
func TestNotificationsConfig_ITermExplicitFalseRoundTrip(t *testing.T) {
	const data = `
[notifications]
iterm_enabled = false
on_needs_input = false
on_finished = true
on_error = false
`
	var cfg UserConfig
	if _, err := toml.Decode(data, &cfg); err != nil {
		t.Fatalf("toml.Decode failed: %v", err)
	}

	n := cfg.Notifications
	if n.GetITermEnabled() {
		t.Error("iterm_enabled=false must be honored (not defaulted to true)")
	}
	if n.GetOnNeedsInput() {
		t.Error("on_needs_input=false must be honored")
	}
	if !n.GetOnFinished() {
		t.Error("on_finished=true must be honored")
	}
	if n.GetOnError() {
		t.Error("on_error=false must be honored")
	}
}

// TestNotificationsConfig_ITermExplicitTrueRoundTrip pins that an explicit
// true survives a TOML round-trip (encode then decode) — guards against a
// field being dropped from the marshaller.
func TestNotificationsConfig_ITermExplicitTrueRoundTrip(t *testing.T) {
	tru := true
	in := UserConfig{
		Notifications: NotificationsConfig{
			ITermEnabled: &tru,
			OnNeedsInput: &tru,
			OnFinished:   &tru,
			OnError:      &tru,
		},
	}

	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(in); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	var out UserConfig
	if _, err := toml.Decode(buf.String(), &out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	n := out.Notifications
	if !n.GetITermEnabled() || !n.GetOnNeedsInput() || !n.GetOnFinished() || !n.GetOnError() {
		t.Errorf("explicit-true flags must survive round-trip; got %+v", n)
	}
}
