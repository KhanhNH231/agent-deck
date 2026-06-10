# Classify + order projects by status

**Date:** 2026-06-10
**Branch:** feat/agent-deck-session-ux
**Status:** Approved, implemented

## Goal

Let the left-list order projects (root groups) by how much they need attention,
and surface each project's status at a glance.

## Behavior

### Order — toggle, default manual

New per-session toggle `projectSortMode` (manual | status), persisted in
`uiState` (`project_sort_mode`). Hotkey `project_sort` (default `o`) cycles it
and shows a status banner ("Projects: by status" / "Projects: manual order").

- **manual** (default): unchanged from today — user-customized group `Order`.
- **status**: root-group blocks reordered so the most-actionable project comes
  first, using the existing actionable ranking
  (`error < waiting < running < idle < stopped`). A project's rank = the most
  urgent (lowest) `ActionablePriority` among its sessions.

The reorder is **UI-only** (`reorderRootGroupBlocksByStatus` in
`project_sort.go`): it rearranges contiguous root-group blocks in `flatItems`
during `rebuildFlatItems`, before remotes are appended and before `RootGroupNum`
is assigned (so 1-9 hotkeys match the displayed order). The persisted group
`Order` and the web view are untouched.

- `conductor` stays pinned at the top.
- Sort is **stable** → equal-status projects keep their manual order as tiebreak.
- Cursor is preserved across the toggle via
  `captureSelectedItemIdentity` / `rebuildFlatItemsPreservingSelection`.

### Classify — error indicator

Project headers already show `● N` (running) and `◐ N` (waiting). Added `✕ N`
(red, `GroupStatusError`) for errored sessions, computed in
`buildGroupRenderStats` (recursive over descendants like the others).

## Liveness

Order updates on each `rebuildFlatItems` (fires on session-load / status events),
not on every sub-second status flicker. This is intentional: it avoids yanking
the cursor while the user reads.

## New export

`session.ActionablePriority(Status) int` — thin exported wrapper over the
existing unexported `actionablePriority`, so the UI can rank without duplicating
the table.

## Files

- `internal/session/groups.go` — export `ActionablePriority`.
- `internal/ui/project_sort.go` (new) — mode type, `reorderRootGroupBlocksByStatus`.
- `internal/ui/home.go` — `projectSortMode` field; reorder hook in
  `rebuildFlatItems`; `case "o"` toggle; `uiState` persist; `groupRenderStats.errored`
  + header render.
- `internal/ui/hotkeys.go` — register `project_sort` (default `o`).
- `internal/ui/help.go` — NAVIGATION help entry.
- `internal/ui/styles.go` — `GroupStatusError` (red).
- `internal/ui/project_sort_test.go` (new).

## Tests

- mode `next()` / `label()`.
- reorder: most-actionable-first; blocks stay contiguous; conductor pinned;
  stable tiebreak for equal priority; pre-root prefix preserved.
- regression: hotkey/keyboard/help/group-render suites still pass.
