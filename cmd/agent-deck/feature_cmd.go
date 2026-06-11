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
			fmt.Fprintln(os.Stderr, "Usage: agent-deck feature extend <name> <repo> [--base ref]\n  <repo> is a [[repos]] manifest name or a path to a git repo")
			os.Exit(1)
		}
		repoName, repoPath, baseRef := repoArg, repoArg, *base
		if def, ok := session.FindManifestRepo(repoArg); ok {
			repoPath = def.PathExpanded()
			if baseRef == "" {
				baseRef = def.DefaultBase
			}
		} else {
			repoName = filepath.Base(repoArg)
		}
		db := openDB()
		defer db.Close()
		if err := session.ExtendFeature(db, name, repoName, repoPath, baseRef); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Extended %q with %s. Restart the feature's session(s) to pick up the new repo.\n", name, repoName)

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
  agent-deck feature list                 List features and their state
  agent-deck feature park <name> [--force]   Remove worktrees, keep docs+branches
  agent-deck feature resume <name>        Recreate worktrees from branches
  agent-deck feature extend <name> <repo> [--base ref]   Add a repo (manifest name or path)
  agent-deck feature conductor <name>     Scaffold contracts/+briefs and set up conductor-<name>
  agent-deck feature delete <name>        Park + remove feature dir (branches kept)

Features are registered automatically when a worktree session is created
while [workspace].root is set in ~/.agent-deck/config.toml.
`)
}
