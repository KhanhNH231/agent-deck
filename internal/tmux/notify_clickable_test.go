//go:build !windows

package tmux

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildTerminalNotifierArgs_Argv pins the exact argv passed to
// terminal-notifier. The click action must run `<binPath> focus <sessionID>`
// with each token a DISCRETE argv element (terminal-notifier -execute runs the
// string via a shell, so we keep the binary path quoted as one argv element and
// the sessionID — internal hex-dash — as its own element; there is no
// concatenation that could let a metachar in the message break out).
func TestBuildTerminalNotifierArgs_Argv(t *testing.T) {
	args := buildTerminalNotifierArgs(
		"Agent Deck",
		"build needs input",
		"sess-abcd-1234",
		"/usr/local/bin/agent-deck",
	)

	// -title / -message carry the user-facing text.
	require.Equal(t, "Agent Deck", argValue(t, args, "-title"))
	require.Equal(t, "build needs input", argValue(t, args, "-message"))

	// iTerm bundle id is reused for both notification style and click-front.
	require.Equal(t, "com.googlecode.iterm2", argValue(t, args, "-sender"))
	require.Equal(t, "com.googlecode.iterm2", argValue(t, args, "-activate"))

	// The click action: `<binPath> focus <sessionID>`.
	require.Equal(t, "/usr/local/bin/agent-deck focus sess-abcd-1234",
		argValue(t, args, "-execute"))
}

// TestBuildTerminalNotifierArgs_SessionIDDiscreteInExecute asserts the sessionID
// appears verbatim in the -execute value and that the binPath precedes the
// literal `focus` subcommand — i.e. argv is assembled in the documented order,
// not garbled.
func TestBuildTerminalNotifierArgs_SessionIDDiscreteInExecute(t *testing.T) {
	args := buildTerminalNotifierArgs("t", "m", "id-9f8e", "agent-deck")
	require.Equal(t, "agent-deck focus id-9f8e", argValue(t, args, "-execute"))
}

// argValue returns the argv element immediately following flag in args, failing
// the test if the flag is absent or terminal.
func argValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i, a := range args {
		if a == flag {
			require.Less(t, i+1, len(args), "flag %q has no value", flag)
			return args[i+1]
		}
	}
	t.Fatalf("flag %q not present in argv %v", flag, args)
	return ""
}

// TestShouldUseClickableNotification_FallbackWhenUnavailable asserts the decider
// returns false (caller falls back to OSC 9) when terminal-notifier is not on
// PATH, regardless of the other gates.
func TestShouldUseClickableNotification_FallbackWhenUnavailable(t *testing.T) {
	require.False(t,
		shouldUseClickableNotification(true /*itermActive*/, true /*clickEnabled*/, false /*available*/),
		"terminal-notifier absent must force the OSC 9 fallback")
}

// TestShouldUseClickableNotification_FallbackWhenDisabled asserts a disabled
// click-action config forces the OSC 9 fallback even when everything else is
// present.
func TestShouldUseClickableNotification_FallbackWhenDisabled(t *testing.T) {
	require.False(t,
		shouldUseClickableNotification(true, false /*clickEnabled*/, true),
		"iterm_click_action=false must force the OSC 9 fallback")
}

// TestShouldUseClickableNotification_FallbackWhenNotITerm asserts a non-iTerm
// terminal forces the fallback (the iTerm bundle-id sender/activate would be
// meaningless elsewhere).
func TestShouldUseClickableNotification_FallbackWhenNotITerm(t *testing.T) {
	require.False(t,
		shouldUseClickableNotification(false /*itermActive*/, true, true),
		"non-iTerm must force the OSC 9 fallback")
}

// TestShouldUseClickableNotification_AllGatesOpen asserts the clickable path is
// chosen only when iTerm is active AND click-action is enabled AND
// terminal-notifier is available.
func TestShouldUseClickableNotification_AllGatesOpen(t *testing.T) {
	require.True(t,
		shouldUseClickableNotification(true, true, true),
		"all gates open must select the clickable path")
}

// TestEmitITermNotificationRouted_FallbackWhenClickDisabled asserts the routed
// emitter returns handled=false (so the caller emits OSC 9) when click-action
// is disabled — even on iTerm — without attempting to spawn terminal-notifier.
func TestEmitITermNotificationRouted_FallbackWhenClickDisabled(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")

	handled := EmitITermNotificationRouted("Agent Deck", "build finished", "sess-1", "agent-deck", false /*clickEnabled*/)
	require.False(t, handled,
		"click-action disabled must return handled=false so the caller falls back to OSC 9")
}

// TestEmitITermNotificationRouted_FallbackOutsideITerm asserts the routed
// emitter returns handled=false outside iTerm2, so the caller's OSC 9 path
// (itself a no-op outside iTerm) owns the decision. Never panics.
func TestEmitITermNotificationRouted_FallbackOutsideITerm(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	t.Setenv("ITERM_SESSION_ID", "")
	t.Setenv("LC_TERMINAL", "")

	var handled bool
	require.NotPanics(t, func() {
		handled = EmitITermNotificationRouted("Agent Deck", "msg", "sess-1", "agent-deck", true)
	})
	require.False(t, handled,
		"outside iTerm2 the routed emitter must return handled=false")
}
