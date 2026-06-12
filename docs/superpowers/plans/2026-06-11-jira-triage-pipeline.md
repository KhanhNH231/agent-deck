# JIRA Triage Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A launchd poller that queues JIRA issues newly assigned to me into an inbox file, plus a `/jira-triage` skill that turns each queued issue into a digest — preliminary check before I look.

**Architecture:** Two artifacts, no agent-deck code. (1) `~/.config/jira-poller/poll.sh` polls `GET /rest/api/3/search/jql` every 10 min via launchd, diffs against `state.json`, appends new keys to `inbox.md`, fires a notification. (2) `~/.claude/skills/jira-triage/SKILL.md` reads `inbox.md`, fetches each unchecked issue via the Atlassian MCP, writes a digest, checks the line off. The two communicate only through `inbox.md`'s line format, which is the stable interface.

**Tech Stack:** Bash, `jq`, `curl`, macOS `security` (Keychain), `launchctl`, `terminal-notifier`, Atlassian MCP (skill side). All deps verified present except `shellcheck` (optional).

**Spec:** `docs/superpowers/specs/2026-06-11-jira-triage-pipeline-design.md`

---

## File structure

| File | Responsibility |
|---|---|
| `~/.config/jira-poller/poll.sh` | Fetch assigned issues → diff vs state → append inbox → notify. Network is mockable via `JIRA_FIXTURE` env. |
| `~/.config/jira-poller/test_poll.sh` | Behavior test driving `poll.sh` with fixture JSON through a temp config dir. |
| `~/.config/jira-poller/fixtures/two.json` | 2-issue fixture. |
| `~/.config/jira-poller/fixtures/three.json` | 3-issue fixture (superset of two.json + 1 new). |
| `~/.config/jira-poller/com.khanh.jira-poller.plist` | launchd job: run `poll.sh` every 600s. |
| `~/.config/jira-poller/state.json` | Runtime: `{ "seen": {KEY: ts}, "error_notified": bool }`. Created by script. |
| `~/.config/jira-poller/inbox.md` | Runtime: queue. Created by script. |
| `~/.config/jira-poller/digests/<KEY>.md` | Runtime: written by the skill. |
| `~/.config/jira-poller/poller.log` | Runtime: append-only log. |
| `~/.claude/skills/jira-triage/SKILL.md` | The triage skill. |

**Config dir override:** `poll.sh` honors `JIRA_POLLER_DIR` (default `~/.config/jira-poller`) so the test can point it at a temp dir. All runtime paths derive from it.

---

## Task 1: Poller core — diff + append, mockable via fixture (TDD)

**Files:**
- Create: `~/.config/jira-poller/poll.sh`
- Create: `~/.config/jira-poller/test_poll.sh`
- Create: `~/.config/jira-poller/fixtures/two.json`
- Create: `~/.config/jira-poller/fixtures/three.json`

The fetch boundary is mocked: when `JIRA_FIXTURE` is set, `poll.sh` reads that file instead of calling `curl`. The fixture mirrors the real `/search/jql` response shape so the same `jq` parsing runs in test and prod.

- [ ] **Step 1: Create the two-issue fixture**

`~/.config/jira-poller/fixtures/two.json`:
```json
{
  "isLast": true,
  "issues": [
    { "key": "WMS-101", "fields": { "summary": "Returns flow drops package", "status": {"name": "To Do"}, "priority": {"name": "High"}, "issuetype": {"name": "Bug"} } },
    { "key": "WMS-102", "fields": { "summary": "Multi-package label print", "status": {"name": "In Progress"}, "priority": {"name": "Medium"}, "issuetype": {"name": "Story"} } }
  ]
}
```

- [ ] **Step 2: Create the three-issue fixture (two.json + ADI-7)**

`~/.config/jira-poller/fixtures/three.json`:
```json
{
  "isLast": true,
  "issues": [
    { "key": "WMS-101", "fields": { "summary": "Returns flow drops package", "status": {"name": "To Do"}, "priority": {"name": "High"}, "issuetype": {"name": "Bug"} } },
    { "key": "WMS-102", "fields": { "summary": "Multi-package label print", "status": {"name": "In Progress"}, "priority": {"name": "Medium"}, "issuetype": {"name": "Story"} } },
    { "key": "ADI-7", "fields": { "summary": "Adidas activation webhook", "status": {"name": "To Do"}, "priority": {"name": "Low"}, "issuetype": {"name": "Task"} } }
  ]
}
```

- [ ] **Step 3: Write the failing behavior test**

`~/.config/jira-poller/test_poll.sh`:
```bash
#!/usr/bin/env bash
# Behavior test for poll.sh. Drives the script through a temp config dir with
# fixture JSON (network mocked). Asserts inbox/state through the file interface.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
POLL="$SCRIPT_DIR/poll.sh"
FIX="$SCRIPT_DIR/fixtures"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
export JIRA_POLLER_DIR="$TMP"

fail() { echo "FAIL: $1" >&2; exit 1; }
inbox_lines() { grep -c '^- \[ \] ' "$TMP/inbox.md" 2>/dev/null || echo 0; }

# 1. First run with 2 issues -> 2 queued lines, 2 seen keys.
JIRA_FIXTURE="$FIX/two.json" NO_NOTIFY=1 bash "$POLL"
[ "$(inbox_lines)" -eq 2 ] || fail "expected 2 inbox lines, got $(inbox_lines)"
grep -q 'WMS-101' "$TMP/inbox.md" || fail "WMS-101 missing from inbox"
[ "$(jq '.seen | length' "$TMP/state.json")" -eq 2 ] || fail "expected 2 seen keys"

# 2. Re-run same fixture -> idempotent, still 2 lines.
JIRA_FIXTURE="$FIX/two.json" NO_NOTIFY=1 bash "$POLL"
[ "$(inbox_lines)" -eq 2 ] || fail "re-run not idempotent: $(inbox_lines) lines"

# 3. Run with 3 issues -> exactly 1 new line (ADI-7).
JIRA_FIXTURE="$FIX/three.json" NO_NOTIFY=1 bash "$POLL"
[ "$(inbox_lines)" -eq 3 ] || fail "expected 3 inbox lines, got $(inbox_lines)"
grep -q 'ADI-7' "$TMP/inbox.md" || fail "ADI-7 missing after growth"

echo "PASS: all poll.sh behavior assertions"
```

- [ ] **Step 4: Run the test, verify it fails**

Run: `bash ~/.config/jira-poller/test_poll.sh`
Expected: FAIL — `poll.sh` does not exist yet (`bash: .../poll.sh: No such file or directory`).

- [ ] **Step 5: Write the minimal poll.sh (fixture path + diff/append only)**

`~/.config/jira-poller/poll.sh`:
```bash
#!/usr/bin/env bash
# JIRA assigned-issue poller. Diffs assigned-open issues against state.json,
# appends new keys to inbox.md. Network is mocked when JIRA_FIXTURE is set.
set -euo pipefail

DIR="${JIRA_POLLER_DIR:-$HOME/.config/jira-poller}"
STATE="$DIR/state.json"
INBOX="$DIR/inbox.md"
LOG="$DIR/poller.log"
mkdir -p "$DIR"
[ -f "$STATE" ] || echo '{"seen":{},"error_notified":false}' > "$STATE"
[ -f "$INBOX" ] || printf '# JIRA triage queue\n\n' > "$INBOX"

log() { printf '%s %s\n' "$(date '+%Y-%m-%dT%H:%M:%S')" "$1" >> "$LOG"; }

# fetch_issues: prints the raw /search/jql JSON to stdout.
# Mocked via JIRA_FIXTURE; real implementation added in Task 2.
fetch_issues() {
  if [ -n "${JIRA_FIXTURE:-}" ]; then
    [ -f "$JIRA_FIXTURE" ] || { log "fixture missing: $JIRA_FIXTURE"; return 1; }
    cat "$JIRA_FIXTURE"
    return 0
  fi
  echo "REAL_FETCH_NOT_IMPLEMENTED" >&2
  return 1
}

raw="$(fetch_issues)" || { log "fetch failed"; exit 0; }

now="$(date '+%Y-%m-%d %H:%M')"
new_count=0
# Iterate KEY<TAB>summary. while-read via process substitution keeps new_count
# in the current shell (a pipe would subshell it) and avoids bash-4 `mapfile`
# — the launchd job runs /bin/bash (macOS 3.2).
while IFS=$'\t' read -r key summary; do
  [ -z "$key" ] && continue
  seen="$(jq -r --arg k "$key" '.seen[$k] // empty' "$STATE")"
  if [ -z "$seen" ]; then
    printf -- '- [ ] %s — %s (queued %s)\n' "$key" "$summary" "$now" >> "$INBOX"
    tmp="$(mktemp)"; jq --arg k "$key" --arg t "$now" '.seen[$k]=$t' "$STATE" > "$tmp" && mv "$tmp" "$STATE"
    new_count=$((new_count+1))
    log "queued $key"
  fi
done < <(printf '%s' "$raw" | jq -r '.issues[] | "\(.key)\t\(.fields.summary)"')
log "tick done; $new_count new"
```

- [ ] **Step 6: Make both scripts executable**

Run: `chmod +x ~/.config/jira-poller/poll.sh ~/.config/jira-poller/test_poll.sh`

- [ ] **Step 7: Run the test, verify it passes**

Run: `bash ~/.config/jira-poller/test_poll.sh`
Expected: `PASS: all poll.sh behavior assertions`

- [ ] **Step 8: Commit**

```bash
cd ~/.config/jira-poller && git init -q 2>/dev/null; git add poll.sh test_poll.sh fixtures/
git commit -m "feat(jira-poller): diff/append core with fixture-mocked fetch"
```
(If `~/.config/jira-poller` should not be its own git repo, skip `git init` and instead copy these into a tracked tooling repo. Decide at execution time — see Task 6.)

---

## Task 2: Real JIRA fetch — /search/jql + Keychain + paginate

**Files:**
- Modify: `~/.config/jira-poller/poll.sh` (replace the `REAL_FETCH_NOT_IMPLEMENTED` branch)

- [ ] **Step 1: Replace the real-fetch branch in `fetch_issues()`**

In `poll.sh`, replace these lines:
```bash
  echo "REAL_FETCH_NOT_IMPLEMENTED" >&2
  return 1
```
with:
```bash
  local email="khanh.nguyen@intrepid.asia"
  local base="https://intrepid-asia.atlassian.net"
  local jql="assignee = currentUser() AND statusCategory != Done"
  local token
  token="$(security find-generic-password -s jira-poller -w 2>/dev/null)" || {
    log "keychain token missing (service=jira-poller)"; return 1; }

  local page=0 next="" merged='{"issues":[]}'
  while [ "$page" -lt 5 ]; do
    # Build curl args as an array so the optional nextPageToken arg is added
    # without ${var:+...} word-splitting (bash 3.2 safe).
    local args=( -sS -G "$base/rest/api/3/search/jql"
      -u "$email:$token"
      -H 'Accept: application/json'
      --data-urlencode "jql=$jql"
      --data-urlencode "fields=summary,status,priority,issuetype"
      --data-urlencode "maxResults=100"
      -w $'\n%{http_code}' )
    [ -n "$next" ] && args+=( --data-urlencode "nextPageToken=$next" )
    local resp; resp="$(curl "${args[@]}")" || { log "curl error"; return 1; }
    local code="${resp##*$'\n'}"
    local body="${resp%$'\n'*}"
    if [ "$code" != "200" ]; then
      log "http $code from /search/jql"
      printf '%s' "$body" >&2
      return 2   # distinguish auth/HTTP failure for Task 3 notify gating
    fi
    merged="$(jq -s '{issues: (.[0].issues + .[1].issues)}' \
      <(printf '%s' "$merged") <(printf '%s' "$body"))"
    next="$(printf '%s' "$body" | jq -r '.nextPageToken // empty')"
    local islast; islast="$(printf '%s' "$body" | jq -r '.isLast // true')"
    page=$((page+1))
    { [ "$islast" = "true" ] || [ -z "$next" ]; } && break
  done
  printf '%s' "$merged"
  return 0
```

- [ ] **Step 2: Re-run the fixture test (regression — fixture path untouched)**

Run: `bash ~/.config/jira-poller/test_poll.sh`
Expected: `PASS: all poll.sh behavior assertions`

- [ ] **Step 3: Store the API token in Keychain (one-time, manual)**

Create an Atlassian API token at https://id.atlassian.com/manage-profile/security/api-tokens, then:
```bash
security add-generic-password -s jira-poller -a khanh.nguyen@intrepid.asia -w
```
(Prompts for the token; not echoed. Re-run with `-U` to update.)

- [ ] **Step 4: Manual smoke test against real JIRA**

Run: `JIRA_POLLER_DIR=/tmp/jp-smoke bash ~/.config/jira-poller/poll.sh && cat /tmp/jp-smoke/inbox.md`
Expected: inbox lists your real assigned-open issues; `cat /tmp/jp-smoke/poller.log` shows `tick done; N new`. Run twice → second run adds 0 lines.
**Label "not yet verified" until this output is captured.**

- [ ] **Step 5: Commit**

```bash
cd ~/.config/jira-poller && git add poll.sh
git commit -m "feat(jira-poller): real /search/jql fetch via Keychain token, paginated+capped"
```

---

## Task 3: Error notify gating + per-issue notification

**Files:**
- Modify: `~/.config/jira-poller/poll.sh`
- Modify: `~/.config/jira-poller/test_poll.sh` (add error-gating assertion)

Goal: on auth/HTTP failure, notify **once** (gated on `error_notified` in state), not every 10 min; clear the flag on recovery. On new issues, fire one `terminal-notifier` summary. `NO_NOTIFY=1` suppresses notifications in tests.

- [ ] **Step 1: Add the failing error-gating assertion to the test**

Append before the final `echo "PASS"` in `test_poll.sh`:
```bash
# 4. Fetch failure sets error_notified once; success clears it.
JIRA_FIXTURE="$TMP/does-not-exist.json" NO_NOTIFY=1 bash "$POLL" || true
[ "$(jq -r '.error_notified' "$TMP/state.json")" = "true" ] || fail "error flag not set on fetch failure"
JIRA_FIXTURE="$FIX/two.json" NO_NOTIFY=1 bash "$POLL"
[ "$(jq -r '.error_notified' "$TMP/state.json")" = "false" ] || fail "error flag not cleared on recovery"
```

- [ ] **Step 2: Run the test, verify the new assertion fails**

Run: `bash ~/.config/jira-poller/test_poll.sh`
Expected: FAIL — `error flag not set on fetch failure` (no flag logic yet; missing fixture currently just `exit 0`).

- [ ] **Step 3: Implement notify + error gating in `poll.sh`**

Add this helper after the `log()` definition:
```bash
notify() {
  [ -n "${NO_NOTIFY:-}" ] && return 0
  command -v terminal-notifier >/dev/null || return 0
  terminal-notifier -title "JIRA" -message "$1" -group jira-poller >/dev/null 2>&1 || true
}
set_error_flag() {  # $1 = true|false ; notifies once on the false->true edge
  local cur; cur="$(jq -r '.error_notified' "$STATE")"
  if [ "$1" = "true" ] && [ "$cur" != "true" ]; then
    notify "poller error — check poller.log"
  fi
  local tmp; tmp="$(mktemp)"; jq --argjson v "$1" '.error_notified=$v' "$STATE" > "$tmp" && mv "$tmp" "$STATE"
}
```

Replace the fetch-failure line:
```bash
raw="$(fetch_issues)" || { log "fetch failed"; exit 0; }
```
with:
```bash
if ! raw="$(fetch_issues)"; then
  log "fetch failed"
  set_error_flag true
  exit 0
fi
set_error_flag false
```

At the end of the script, after the `for` loop, add a single summary notification:
```bash
if [ "$new_count" -gt 0 ]; then
  notify "$new_count new issue(s) assigned — run /jira-triage"
fi
```

- [ ] **Step 4: Run the test, verify all assertions pass**

Run: `bash ~/.config/jira-poller/test_poll.sh`
Expected: `PASS: all poll.sh behavior assertions`

- [ ] **Step 5: Commit**

```bash
cd ~/.config/jira-poller && git add poll.sh test_poll.sh
git commit -m "feat(jira-poller): edge-gated error notify + new-issue notification"
```

---

## Task 4: launchd job

**Files:**
- Create: `~/.config/jira-poller/com.khanh.jira-poller.plist`

- [ ] **Step 1: Write the plist**

`~/.config/jira-poller/com.khanh.jira-poller.plist` (replace `USERNAME` with output of `whoami`):
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.khanh.jira-poller</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>/Users/USERNAME/.config/jira-poller/poll.sh</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>StartInterval</key><integer>600</integer>
  <key>RunAtLoad</key><true/>
  <key>StandardErrorPath</key><string>/Users/USERNAME/.config/jira-poller/launchd.err</string>
  <key>StandardOutPath</key><string>/Users/USERNAME/.config/jira-poller/launchd.out</string>
</dict>
</plist>
```
(`PATH` must include `/opt/homebrew/bin` so launchd finds `jq`/`terminal-notifier`.)

- [ ] **Step 2: Load the job (manual)**

```bash
sed -i '' "s/USERNAME/$(whoami)/g" ~/.config/jira-poller/com.khanh.jira-poller.plist
cp ~/.config/jira-poller/com.khanh.jira-poller.plist ~/Library/LaunchAgents/
launchctl unload ~/Library/LaunchAgents/com.khanh.jira-poller.plist 2>/dev/null || true
launchctl load ~/Library/LaunchAgents/com.khanh.jira-poller.plist
```

- [ ] **Step 3: Verify it ran (manual)**

Run: `launchctl list | grep jira-poller && sleep 5 && tail -5 ~/.config/jira-poller/poller.log`
Expected: a list entry, and a recent `tick done` log line. First launchd run may prompt for Keychain access — approve **Always Allow**.
**Label "not yet verified" until captured.**

- [ ] **Step 4: Commit**

```bash
cd ~/.config/jira-poller && git add com.khanh.jira-poller.plist
git commit -m "feat(jira-poller): launchd job, 600s interval"
```

---

## Task 5: jira-triage skill

**Files:**
- Create: `~/.claude/skills/jira-triage/SKILL.md`

- [ ] **Step 1: Write SKILL.md**

`~/.claude/skills/jira-triage/SKILL.md`:
```markdown
---
name: jira-triage
description: Use when the user runs /jira-triage or asks to triage queued JIRA issues. Reads ~/.config/jira-poller/inbox.md, fetches each unchecked issue via the Atlassian MCP, writes a digest to digests/<KEY>.md, and checks the line off. Digest-only — no repo mapping or code recon.
---

# JIRA Triage

Produce a preliminary digest for each queued, unchecked JIRA issue so the user
can decide what to pick up before opening JIRA.

## Inputs
- Queue: `~/.config/jira-poller/inbox.md` — lines `- [ ] KEY — summary (queued ts)`.
- Digests dir: `~/.config/jira-poller/digests/` (create if absent).

## Steps
1. Read `inbox.md`. Collect every line matching `^- \[ \] ([A-Z][A-Z0-9]+-\d+)` —
   the unchecked ones. If none, tell the user the queue is empty and stop.
2. Deduplicate keys (the poller may, rarely, have appended a key twice).
3. For each unique key, call the Atlassian MCP `getJiraIssue` (cloudId for
   `intrepid-asia.atlassian.net`; resolve via `getAccessibleAtlassianResources`
   once and reuse). Request fields: summary, description, status, priority,
   issuetype, reporter, assignee, parent/epic, issuelinks, and recent comments.
4. Write `~/.config/jira-poller/digests/<KEY>.md` containing:
   - Title line: `# <KEY> — <summary>`
   - **Meta:** type, priority, status, reporter, current assignee. If the current
     assignee is no longer the user, add `> ⚠ reassigned away — <name>`.
   - **Acceptance criteria** (from description; verbatim if present, else
     "none stated").
   - **Links / epic:** parent epic and linked issue keys with relationship.
   - **Comments gist:** 1–3 sentence summary of the discussion, or "no comments".
5. After writing each digest, mark its inbox line(s): change `- [ ] KEY` to
   `- [x] KEY`. Mark ALL lines for a deduped key.
6. End with a one-screen summary table: KEY | type/priority | one-line gist |
   `digests/<KEY>.md`.

## Error handling
- Atlassian MCP unavailable or unauthenticated: report it to the user, leave the
  affected lines unchecked (the skill is safely re-runnable), continue with any
  issues that did succeed.
- `getJiraIssue` 404 / no permission: write a stub digest noting the failure,
  still check the line off (it will never succeed on retry).

## Out of scope
Repo/feature mapping, code-level recon, readiness verdict. Digest only.
```

- [ ] **Step 2: Verify the skill is discoverable**

Run: `ls ~/.claude/skills/jira-triage/SKILL.md && head -4 ~/.claude/skills/jira-triage/SKILL.md`
Expected: file exists; frontmatter `name`/`description` present.

- [ ] **Step 3: Manual end-to-end verification**

In a Claude Code session with the Atlassian MCP connected: ensure `inbox.md` has at
least one `- [ ]` line (from Task 2 smoke, or hand-add `- [ ] WMS-1 — test`), invoke
`/jira-triage`, confirm a digest file appears and the inbox line flips to `- [x]`.
**Label "not yet verified" until run.**

- [ ] **Step 4: Commit**

```bash
cd ~/.claude/skills && git add jira-triage/SKILL.md 2>/dev/null && \
  git commit -m "feat(skill): jira-triage digest skill" 2>/dev/null || \
  echo "skills dir not a repo — see Task 6 for placement decision"
```

---

## Task 6: Placement / version-control decision (do first if unsure)

`~/.config/jira-poller` and `~/.claude/skills` may or may not be git-tracked. Before
Task 1's commit step, decide:

- [ ] **Option A — dedicated tooling repo (recommended):** create `~/Documents/Projects/jira-poller/`, develop there, symlink `poll.sh`/plist into `~/.config/jira-poller` and the skill into `~/.claude/skills/jira-triage`. Clean history, backup-able.
- [ ] **Option B — in place, no VCS:** develop directly in `~/.config/jira-poller`; skip every `git` step above. Simplest; no history.
- [ ] **Option C — `~/.claude/skills` is already a git repo:** keep the skill there; put the poller in its own repo (A).

Pick one and adjust the commit steps in Tasks 1–5 accordingly. This task has no code — it's a gate so the commit steps don't fail silently.

---

## Self-review notes

- **Spec coverage:** poll trigger (T1–T2,T4), Keychain auth (T2), full-list diff/no-time-window (T1), inbox format (T1), error notify gating (T3), digest skill + fields + reassigned-away (T5), out-of-scope honored (no conductor/mapping/recon). ✓
- **Endpoint correctness:** uses `/search/jql` + `nextPageToken`, 5-page cap, no `total` reliance — matches corrected spec. ✓
- **Interface stability:** poller↔skill couple only via `inbox.md` line regex `^- \[ \] (KEY)` — used identically in T1 fixture assertions and T5 step 1. ✓
- **Mock boundary:** network mocked via `JIRA_FIXTURE`; test never needs token or network. ✓
- **Known risk carried from spec:** first launchd Keychain access prompt (T4 step 3); two auth surfaces (poller token vs MCP) — accepted in spec.
- **bash 3.2 compatibility:** launchd runs `/bin/bash` (macOS 3.2) — no `mapfile`, no `${var:+...}` for multi-arg; uses `while read` + curl args array. The test runs under whatever `bash` is on PATH (homebrew 5) but the script stays 3.2-clean. Optional belt-and-suspenders: run the test once with `/bin/bash test_poll.sh` too.
