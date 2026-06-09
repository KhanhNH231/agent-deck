package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsNewFocusRequest pins the "newer request" predicate the poller uses to
// avoid re-firing on a stale focus_request.json: a request whose timestamp is
// <= the last handled one must NOT re-fire; a strictly-newer one must.
func TestIsNewFocusRequest(t *testing.T) {
	require.False(t, isNewFocusRequest(100, 100),
		"equal timestamp (already handled) must not re-fire")
	require.False(t, isNewFocusRequest(100, 150),
		"older-than-last timestamp must not re-fire")
	require.True(t, isNewFocusRequest(200, 150),
		"strictly-newer timestamp must fire")
	require.True(t, isNewFocusRequest(1, 0),
		"first request (lastHandled zero) must fire")
}
