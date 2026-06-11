package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScaffoldFeatureDocs creates the feature's doc skeleton under featureDir:
//
//	CLAUDE.md          — feature context (repo table, layout, lifecycle hints)
//	docs/handoff.md    — running summary stub, survives park
//	contracts/         — cross-repo interface notes (conductor mode only)
//	workers/<repo>.md  — per-repo worker briefs (conductor mode only)
//
// Existing files are never overwritten, so re-registering or resuming a
// feature preserves user edits.
func ScaffoldFeatureDocs(featureDir, name string, repos []FeatureRepo, conductor bool) error {
	if err := os.MkdirAll(filepath.Join(featureDir, "docs"), 0o755); err != nil {
		return fmt.Errorf("scaffold %q: %w", name, err)
	}

	if err := writeIfAbsent(filepath.Join(featureDir, "CLAUDE.md"), featureClaudeMD(name, repos, conductor)); err != nil {
		return err
	}
	if err := writeIfAbsent(filepath.Join(featureDir, "docs", "handoff.md"),
		fmt.Sprintf("# Handoff: %s\n\nRunning summary of this feature. Survives `agent-deck feature park`.\n", name)); err != nil {
		return err
	}

	if conductor {
		if err := os.MkdirAll(filepath.Join(featureDir, "contracts"), 0o755); err != nil {
			return fmt.Errorf("scaffold %q: %w", name, err)
		}
		if err := writeIfAbsent(filepath.Join(featureDir, "contracts", "README.md"),
			"# Contracts\n\nCross-repo interfaces agreed between workers. One file per contract.\n"); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(featureDir, "workers"), 0o755); err != nil {
			return fmt.Errorf("scaffold %q: %w", name, err)
		}
		for _, r := range repos {
			brief := fmt.Sprintf(`# Worker brief: %s

Feature: %s
Worktree: worktrees/%s (branch %s)

Scope: changes in this repo only. Cross-repo interface changes go through
contracts/ and the conductor.
`, r.RepoName, name, r.RepoName, r.Branch)
			if err := writeIfAbsent(filepath.Join(featureDir, "workers", r.RepoName+".md"), brief); err != nil {
				return err
			}
		}
	}
	return nil
}

func featureClaudeMD(name string, repos []FeatureRepo, conductor bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Feature: %s\n\n", name)
	b.WriteString("This directory is a managed agent-deck workspace feature.\n\n")
	b.WriteString("## Repos\n\n| Repo | Branch | Base | Worktree |\n|---|---|---|---|\n")
	for _, r := range repos {
		base := r.BaseRef
		if base == "" {
			base = "-"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | worktrees/%s |\n", r.RepoName, r.Branch, base, r.RepoName)
	}
	b.WriteString(`
## Layout

- worktrees/<repo>/ — isolated git worktrees; all code changes happen here
- docs/handoff.md   — running summary; keep it current, it survives park
`)
	if conductor {
		b.WriteString(`- contracts/        — agreed cross-repo interfaces
- workers/<repo>.md — per-repo worker briefs
`)
	}
	b.WriteString(`
## Lifecycle

- Park (worktrees removed, branches+docs kept): agent-deck feature park ` + name + `
- Resume: agent-deck feature resume ` + name + `
- Add a repo: agent-deck feature extend ` + name + ` <repo>
`)
	return b.String()
}

// writeIfAbsent writes content to path unless the file already exists.
func writeIfAbsent(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
