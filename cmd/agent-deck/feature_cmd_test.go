package main

import "testing"

func TestSplitRepoBranch(t *testing.T) {
	tests := []struct {
		arg        string
		wantRepo   string
		wantBranch string
	}{
		// No "@" → plain repo name, empty branch.
		{arg: "my-repo", wantRepo: "my-repo", wantBranch: ""},
		// Simple "repo@branch" with slash in branch.
		{arg: "my-repo@feat/x", wantRepo: "my-repo", wantBranch: "feat/x"},
		// Path separator in repo portion, slash in branch.
		{arg: "path/to/repo@feat/x", wantRepo: "path/to/repo", wantBranch: "feat/x"},
		// "@" present but branch part is empty — treat as empty branch.
		{arg: "repo@", wantRepo: "repo", wantBranch: ""},
		// Documented limitation: repo path containing "@" splits on the FIRST "@".
		// "we@ird/path@feat/x" → repo="we", branch="ird/path@feat/x".
		{arg: "we@ird/path@feat/x", wantRepo: "we", wantBranch: "ird/path@feat/x"},
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			gotRepo, gotBranch := splitRepoBranch(tt.arg)
			if gotRepo != tt.wantRepo {
				t.Errorf("repo: got %q, want %q", gotRepo, tt.wantRepo)
			}
			if gotBranch != tt.wantBranch {
				t.Errorf("branch: got %q, want %q", gotBranch, tt.wantBranch)
			}
		})
	}
}
