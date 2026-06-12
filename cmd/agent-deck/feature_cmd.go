package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/asheshgoplani/agent-deck/internal/session"
	"github.com/asheshgoplani/agent-deck/internal/statedb"
)

// handleFeature implements `agent-deck feature <list|park|resume|delete>`:
// the workspace-feature lifecycle (wsw absorption).
func handleFeature(profile string, args []string) {
	if len(args) == 0 {
		printFeatureHelp()
		os.Exit(1)
	}

	openDB := func() *statedb.StateDB {
		dbPath, err := session.GetDBPathForProfile(profile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: resolve db path: %v\n", err)
			os.Exit(1)
		}
		db, err := statedb.Open(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: open statedb: %v\n", err)
			os.Exit(1)
		}
		if err := db.Migrate(); err != nil {
			db.Close()
			fmt.Fprintf(os.Stderr, "Error: migrate statedb: %v\n", err)
			os.Exit(1)
		}
		return db
	}

	switch args[0] {
	case "list", "ls":
		db := openDB()
		defer db.Close()
		features, err := db.ListFeatures()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: list features: %v\n", err)
			os.Exit(1)
		}
		if len(features) == 0 {
			fmt.Println("No features. Worktree sessions register one automatically when [workspace] is configured.")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSTATE\tREPOS\tROOT")
		for _, f := range features {
			_, repos, _ := db.GetFeatureByID(f.ID)
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", f.Name, f.State, len(repos), f.RootPath)
		}
		w.Flush()

	case "park", "stop":
		fs := flag.NewFlagSet("feature park", flag.ExitOnError)
		force := fs.Bool("force", false, "park even with uncommitted changes")
		_ = fs.Parse(args[1:])
		name := fs.Arg(0)
		if name == "" {
			fmt.Fprintln(os.Stderr, "Usage: agent-deck feature park <name> [--force]")
			os.Exit(1)
		}
		db := openDB()
		defer db.Close()
		if err := session.ParkFeature(db, name, *force); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Parked %q — worktrees removed, docs and branches kept. Resume with: agent-deck feature resume %s\n", name, name)

	case "resume":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		if name == "" {
			fmt.Fprintln(os.Stderr, "Usage: agent-deck feature resume <name>")
			os.Exit(1)
		}
		db := openDB()
		defer db.Close()
		if err := session.ResumeFeature(db, name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Resumed %q — worktrees recreated from their branches.\n", name)

	case "extend":
		fs := flag.NewFlagSet("feature extend", flag.ExitOnError)
		base := fs.String("base", "", "base ref for the new branch (default: repo's manifest default_base, else current HEAD)")
		_ = fs.Parse(args[1:])
		name, repoArg := fs.Arg(0), fs.Arg(1)
		if name == "" || repoArg == "" {
			fmt.Fprintln(os.Stderr, "Usage: agent-deck feature extend <name> <repo>[@branch] [--base ref]\n  <repo> is a [[repos]] manifest name or a path to a git repo\n  @branch puts the new worktree on a specific branch (branch may contain \"/\")")
			os.Exit(1)
		}
		rawRepo, branchArg := splitRepoBranch(repoArg)
		repoName, repoPath, baseRef := rawRepo, rawRepo, *base
		if def, ok := session.FindManifestRepo(rawRepo); ok {
			repoPath = def.PathExpanded()
			if baseRef == "" {
				baseRef = def.DefaultBase
			}
		} else {
			repoName = filepath.Base(rawRepo)
		}
		db := openDB()
		defer db.Close()
		branch := branchArg
		if branch == "" {
			// No explicit branch in the CLI arg: derive from the feature's
			// existing repos so the default is visible at the call site.
			_, repos, err := db.GetFeatureByName(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			branch = session.DefaultFeatureBranch(repos, name)
		}
		warning, err := session.ExtendFeature(db, name, repoName, repoPath, baseRef, branch)
		// Print the warning before any error: a failed fetch often explains
		// why the extend itself failed.
		if warning != "" {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Extended %q with %s on branch %s. Restart the feature's session(s) to pick up the new repo.\n", name, repoName, branch)

	case "delete", "rm":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		if name == "" {
			fmt.Fprintln(os.Stderr, "Usage: agent-deck feature delete <name>")
			os.Exit(1)
		}
		db := openDB()
		defer db.Close()
		if err := session.DeleteFeature(db, name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted %q — feature dir removed; branches kept in their repos.\n", name)

	case "update":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		if name == "" {
			fmt.Fprintln(os.Stderr, "Usage: agent-deck feature update <name>")
			os.Exit(1)
		}
		db := openDB()
		defer db.Close()
		updates, err := session.UpdateFeature(db, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "REPO\tOUTCOME\tCHANGE\tDETAIL")
		for _, u := range updates {
			change := "-"
			if u.Result.OldTip != "" && u.Result.NewTip != "" {
				change = u.Result.OldTip + ".." + u.Result.NewTip
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.RepoName, u.Result.Outcome, change, u.Result.Detail)
		}
		w.Flush()

	case "conductor":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		if name == "" {
			fmt.Fprintln(os.Stderr, "Usage: agent-deck feature conductor <name>")
			os.Exit(1)
		}
		db := openDB()
		f, err := func() (statedb.FeatureRow, error) {
			defer db.Close()
			return session.MarkFeatureConductor(db, name)
		}()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Feature %q is now conductor-orchestrated: contracts/ and workers/ briefs scaffolded under %s\n", name, f.RootPath)
		// Reuse the existing conductor infrastructure; the feature CLAUDE.md
		// becomes the conductor's identity document.
		conductorName := "conductor-" + strings.ReplaceAll(name, "/", "-")
		fmt.Printf("Setting up conductor %q...\n", conductorName)
		handleConductorSetup(profile, []string{
			conductorName,
			"-claude-md", f.RootPath + "/CLAUDE.md",
			"-description", "Conductor for workspace feature " + name,
		})

	case "help", "--help", "-h":
		printFeatureHelp()

	default:
		fmt.Fprintf(os.Stderr, "Unknown feature command: %s\n\n", args[0])
		printFeatureHelp()
		os.Exit(1)
	}
}

func printFeatureHelp() {
	fmt.Print(`Workspace features — park/resume units of work under [workspace].root

Usage:
  agent-deck feature list                           List features and their state
  agent-deck feature park <name> [--force]          Remove worktrees, keep docs+branches
  agent-deck feature resume <name>                  Recreate worktrees from branches
  agent-deck feature update <name>                  Fetch + fast-forward every repo in the feature
                                                    (ff-only, clean-worktree gated; skips reported
                                                    as outcomes, not errors)
  agent-deck feature extend <name> <repo>[@branch] [--base ref]
                                                    Add a repo (manifest name or path).
                                                    @branch puts the new worktree on a specific
                                                    branch (may contain "/", e.g. feat/x).
                                                    Without @branch the feature's existing branch
                                                    is used as the default.
                                                    Note: repo paths containing "@" must use the
                                                    explicit @branch suffix to disambiguate.
  agent-deck feature conductor <name>               Scaffold contracts/+briefs and set up conductor-<name>
  agent-deck feature delete <name>                  Park + remove feature dir (branches kept)

Features are registered automatically when a worktree session is created
while [workspace].root is set in ~/.agent-deck/config.toml.
`)
}

// splitRepoBranch splits "repo@branch". repo may be a path; branch may
// contain "/" (feat/x). Split on the FIRST "@" only; no "@" → branch "".
// Limitation (documented in help): repo paths containing "@" need the
// explicit @branch suffix to disambiguate (first "@" is treated as the
// separator, so "we@ird/path@feat/x" yields repo="we", branch="ird/path@feat/x").
func splitRepoBranch(arg string) (repo, branch string) {
	idx := strings.Index(arg, "@")
	if idx < 0 {
		return arg, ""
	}
	return arg[:idx], arg[idx+1:]
}
