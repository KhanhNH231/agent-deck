# Ctrl+E — back to most recent project (alt-tab)

**Date:** 2026-06-10
**Branch:** feat/agent-deck-session-ux
**Status:** Approved, implemented

## Goal

One keypress (Ctrl+E) jumps the cursor to the project you were last working in,
toggling between the last two — like "go to last file" in an editor.

## Definition

"Project" = the **root group** (Level-0 folder) a session lives under — the same
unit the `1-9` quick-jump keys target. Ungrouped sessions fall back to their
`ProjectPath`.

## Approach — stateless, recency-driven

No focus-history state to maintain. Ctrl+E jumps to the **most-recently-worked
session whose project differs from the cursor's current project**, reusing the
recent-switcher's `sortSessionsByRecency` (keyed on `LastAccessedAt`).

This yields true alt-tab (last-two) behavior for free: recency only advances on
**attach**, not on cursor movement, so bouncing the cursor A→B→A→B with Ctrl+E
keeps the target stable.

- No other project present → no-op + status banner "No other project to switch to".
- Reveal uses the existing `jumpToSession(inst)` (expands the group + parents,
  rebuilds flatItems, moves cursor) so the target is always visible.

## Key binding

`ctrl+e` was previously a hardcoded shortcut to open the feedback dialog on
demand. Decision: **drop that manual shortcut** (the dialog still auto-appears on
its own schedule) and reassign `ctrl+e` to this navigation. Registered as a
rebindable hotkey `recent_project` (default `ctrl+e`) so it appears in `?` help
and respects user overrides.

## Files

- `internal/ui/recent_project.go` (new): `topLevelGroup`, `projectKeyOf`,
  `cursorProjectKey`, `recentProjectTarget`.
- `internal/ui/home.go`: `case "ctrl+e"` body replaced (feedback → recent-project).
- `internal/ui/hotkeys.go`: register `recent_project` action + default binding.
- `internal/ui/help.go`: NAVIGATION help entry.
- `internal/ui/recent_project_test.go` (new): pure-function + target-selection tests.

## Tests

- `topLevelGroup` / `projectKeyOf` mapping (grouped, ungrouped, nested).
- `recentProjectTarget` picks most-recent cross-project session; skips
  same-project; returns nil when only one project exists.
