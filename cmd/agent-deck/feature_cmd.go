package main

import (
	"flag"
	"fmt"
	"os"
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
  agent-deck feature delete <name>        Park + remove feature dir (branches kept)

Features are registered automatically when a worktree session is created
while [workspace].root is set in ~/.agent-deck/config.toml.
`)
}
