package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingWalkthroughs(t *testing.T) {
	tickets := []*Ticket{
		{ID: "01a", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "01b", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "02a", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "02b", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "03a", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "03b", Status: StatusReadyForAgent, PR: "#3"},
		{ID: "04", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "05", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "06", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "07a", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "07b", Status: StatusReadyForAgent, Merged: "historical"},
	}
	committed := []string{
		"docs/代码走读/01-base.md",
		"docs/代码走读/04/nested.md",   // nested files do not count
		"docs/代码走读/05-base.md.bak", // not a markdown file
		"docs/代码走读/06-base.md",
		"docs/代码走读/007-base.md", // not group 07
		"other/08-base.md",      // wrong directory
	}

	missing, checked := missingWalkthroughs(tickets, defaultWalkthroughDocs, committed)
	if got, want := strings.Join(missing, ","), "02,04,05,07"; got != want {
		t.Fatalf("missing = %q, want %q", got, want)
	}
	if checked != 6 {
		t.Fatalf("checked = %d, want 6 complete groups", checked)
	}
}

func TestMissingWalkthroughsAllCovered(t *testing.T) {
	tickets := []*Ticket{
		{ID: "02a", Status: StatusReadyForAgent, Merged: "historical"},
		{ID: "02b", Status: StatusReadyForAgent, Merged: "historical"},
	}
	missing, checked := missingWalkthroughs(tickets, defaultWalkthroughDocs, []string{
		"docs/代码走读/02-interfaces.md",
	})
	if len(missing) != 0 || checked != 1 {
		t.Fatalf("missing = %v, checked = %d, want none and 1", missing, checked)
	}
}

func TestCommittedFilesIgnoresUntracked(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")

	docPath := filepath.Join(root, defaultWalkthroughDocs, "02-store.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("# draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := committedFiles(root, defaultWalkthroughDocs)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("untracked file counted as committed: %v", files)
	}

	runGit(t, root, "add", "-A")
	runGit(t, root, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "init")

	files, err = committedFiles(root, defaultWalkthroughDocs)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.ToSlash(filepath.Join(defaultWalkthroughDocs, "02-store.md"))
	if len(files) != 1 || files[0] != want {
		t.Fatalf("committed files = %v, want [%s]", files, want)
	}
}

func TestVerifyWalkthroughCommand(t *testing.T) {
	root := t.TempDir()
	ticketDir := filepath.Join(root, ".scratch", "collector")
	writeIssue(t, ticketDir, "02a-first.md", "# 02a: First\n\n**Status:** ready-for-agent\n\n**Merged:** historical\n")
	writeIssue(t, ticketDir, "02b-second.md", "# 02b: Second\n\n**Status:** ready-for-agent\n\n**Merged:** historical\n\n**Blocked by:** 02a\n")
	runGit(t, root, "init", "-q")
	runGit(t, root, "add", "-A")
	runGit(t, root, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "init")

	err := cmdVerifyWalkthrough([]string{"--dir", ticketDir, "--root", root})
	if err == nil {
		t.Fatal("verify-walkthrough passed without a committed walkthrough")
	}
	if !strings.Contains(err.Error(), "02") {
		t.Fatalf("error = %v, want it to name group 02", err)
	}

	docPath := filepath.Join(root, defaultWalkthroughDocs, "02-store.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("# walkthrough\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "docs")

	if err := cmdVerifyWalkthrough([]string{"--dir", ticketDir, "--root", root}); err != nil {
		t.Fatalf("verify-walkthrough after commit: %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
