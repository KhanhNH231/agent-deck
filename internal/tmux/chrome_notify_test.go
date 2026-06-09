//go:build !windows

package tmux

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFormatITermNotificationOSC_RoundTrip pins the on-the-wire byte format of
// the iTerm2 OSC 9 (post notification) sequence: ESC ] 9 ; <message> BEL.
// Both the direct-writer and via-tty paths share this string, so a regression
// here breaks both.
func TestFormatITermNotificationOSC_RoundTrip(t *testing.T) {
	require.Equal(t,
		"\x1b]9;Agent Deck: build needs input\a",
		formatITermNotificationOSC("Agent Deck: build needs input"),
		"formatITermNotificationOSC must produce ESC ]9;<message>BEL")

	require.Equal(t, "\x1b]9;\a", formatITermNotificationOSC(""),
		"empty message must still produce a well-formed (empty-payload) OSC 9")
}

// TestFormatITermNotificationOSCViaTmux_DCSEnvelope pins the tmux DCS
// passthrough wrapping rule, mirroring the badge variant: the inner OSC's lone
// ESC is doubled (\x1b\x1b) so tmux strips one and forwards the other.
func TestFormatITermNotificationOSCViaTmux_DCSEnvelope(t *testing.T) {
	got := formatITermNotificationOSCViaTmux("hello")

	want := "\x1bPtmux;\x1b\x1b]9;hello\x07\x1b\\"
	require.Equal(t, want, got,
		"DCS-wrapped form must be ESC P tmux ; ESC ESC <OSC 9 inner> ESC \\")

	require.True(t, strings.HasPrefix(got, "\x1bPtmux;"),
		"must open with the DCS tmux prefix")
	require.True(t, strings.HasSuffix(got, "\x1b\\"),
		"must close with the ST terminator (ESC \\)")
}

// TestEmitITermNotification_WritesOSC9 asserts the writer-based emit produces
// exactly the OSC 9 sequence when iTerm2 is active and the feature is enabled.
func TestEmitITermNotification_WritesOSC9(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("AGENTDECK_ITERM_NOTIFY", "")

	var buf bytes.Buffer
	emitITermNotification(&buf, "Agent Deck: build finished", true)

	require.Equal(t, "\x1b]9;Agent Deck: build finished\a", buf.String(),
		"emitITermNotification must write OSC 9;<message>BEL")
}

// TestEmitITermNotification_NoOpOutsideITerm2 ensures we never write iTerm2
// OSC sequences to terminals that won't parse them.
func TestEmitITermNotification_NoOpOutsideITerm2(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	t.Setenv("ITERM_SESSION_ID", "")
	t.Setenv("LC_TERMINAL", "")
	t.Setenv("AGENTDECK_ITERM_NOTIFY", "")

	var buf bytes.Buffer
	emitITermNotification(&buf, "anything", true)

	require.Empty(t, buf.Bytes(),
		"emitITermNotification must not write outside iTerm2; got %q", buf.String())
}

// TestEmitITermNotification_ConfigDisabledSuppresses covers the
// iterm_enabled=false default path: with env unset, configEnabled=false must
// suppress the write.
func TestEmitITermNotification_ConfigDisabledSuppresses(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("AGENTDECK_ITERM_NOTIFY", "")

	var buf bytes.Buffer
	emitITermNotification(&buf, "msg", false)

	require.Empty(t, buf.Bytes(),
		"configEnabled=false must suppress emits even on iTerm2; got %q", buf.String())
}

// TestEmitITermNotification_EnvForceEnableOverridesConfigOff pins the tri-state
// override: AGENTDECK_ITERM_NOTIFY=1|true|... forces on even when config is off.
func TestEmitITermNotification_EnvForceEnableOverridesConfigOff(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")

	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("AGENTDECK_ITERM_NOTIFY", v)

			var buf bytes.Buffer
			emitITermNotification(&buf, "msg", false)

			require.NotEmpty(t, buf.Bytes(),
				"AGENTDECK_ITERM_NOTIFY=%q must force-enable when config is off", v)
		})
	}
}

// TestEmitITermNotification_EnvForceDisableOverridesConfigOn pins the inverse:
// AGENTDECK_ITERM_NOTIFY=0|false|... forces off even when config is on.
func TestEmitITermNotification_EnvForceDisableOverridesConfigOn(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")

	for _, v := range []string{"0", "false", "FALSE", "no", "off"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("AGENTDECK_ITERM_NOTIFY", v)

			var buf bytes.Buffer
			emitITermNotification(&buf, "msg", true)

			require.Empty(t, buf.Bytes(),
				"AGENTDECK_ITERM_NOTIFY=%q must force-disable when config is on; got %q",
				v, buf.String())
		})
	}
}

// TestEmitITermNotification_EnvGarbageDefersToConfig pins fail-open: an
// unrecognised env value defers to configEnabled instead of flipping it.
func TestEmitITermNotification_EnvGarbageDefersToConfig(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("AGENTDECK_ITERM_NOTIFY", "maybe")

	var bufOff bytes.Buffer
	emitITermNotification(&bufOff, "msg", false)
	require.Empty(t, bufOff.Bytes(),
		"garbage env value with config off must remain off; got %q", bufOff.String())

	var bufOn bytes.Buffer
	emitITermNotification(&bufOn, "msg", true)
	require.NotEmpty(t, bufOn.Bytes(),
		"garbage env value with config on must remain on")
}

// TestEmitITermNotificationViaTty_NoOpOutsideITerm2 ensures the tty emit is a
// safe no-op outside iTerm2 (never opens /dev/tty / never panics).
func TestEmitITermNotificationViaTty_NoOpOutsideITerm2(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	t.Setenv("ITERM_SESSION_ID", "")
	t.Setenv("LC_TERMINAL", "")
	t.Setenv("AGENTDECK_ITERM_NOTIFY", "")

	require.NotPanics(t, func() {
		EmitITermNotificationViaTty("anything", true)
	}, "EmitITermNotificationViaTty must be a silent no-op outside iTerm2")
}
