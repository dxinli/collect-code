package main

import (
	"flag"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const defaultWalkthroughDocs = "docs/代码走读"

func cmdVerifyWalkthrough(args []string) error {
	fs := flag.NewFlagSet("verify-walkthrough", flag.ContinueOnError)
	dir := fs.String("dir", ".scratch/collector", "feature directory")
	feature := fs.String("feature", "", "shorthand for --dir .scratch/<feature>")
	root := fs.String("root", ".", "repository root")
	docs := fs.String("docs", defaultWalkthroughDocs, "walkthrough directory, relative to root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	g, err := LoadGraph(resolveDir(*dir, *feature))
	if err != nil {
		return err
	}
	committed, err := committedFiles(*root, *docs)
	if err != nil {
		return err
	}
	missing, checked := missingWalkthroughs(g.Tickets, *docs, committed)
	if len(missing) > 0 {
		return fmt.Errorf("complete tickets lack a committed walkthrough under %s: %s", *docs, strings.Join(missing, ", "))
	}
	fmt.Printf("walkthroughs: ok (%d complete tickets checked)\n", checked)
	return nil
}

// missingWalkthroughs returns the numeric ticket groups that are fully done
// but have no committed <NN>-*.md file directly under docsDir. A group is the
// numeric prefix shared by sub-tickets (01a/01b/01c, 02a/02b, 06a/06b); a
// single ticket such as 07 forms its own group.
func missingWalkthroughs(tickets []*Ticket, docsDir string, committed []string) ([]string, int) {
	groups := make(map[int][]*Ticket)
	for _, t := range tickets {
		n, _ := splitID(t.ID)
		groups[n] = append(groups[n], t)
	}
	prefix := strings.TrimSuffix(filepath.ToSlash(docsDir), "/") + "/"
	missing := make([]string, 0)
	checked := 0
	for n, members := range groups {
		done := true
		for _, member := range members {
			if member.Status != StatusDone {
				done = false
				break
			}
		}
		if !done {
			continue
		}
		checked++
		groupPrefix := fmt.Sprintf("%02d-", n)
		found := false
		for _, file := range committed {
			if !strings.HasPrefix(file, prefix) {
				continue
			}
			rest := strings.TrimPrefix(file, prefix)
			if strings.Contains(rest, "/") || !strings.HasSuffix(rest, ".md") {
				continue
			}
			if strings.HasPrefix(rest, groupPrefix) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, fmt.Sprintf("%02d", n))
		}
	}
	sort.Slice(missing, func(i, j int) bool { return idLess(missing[i], missing[j]) })
	return missing, checked
}

// committedFiles lists tracked files under docsDir. Untracked walkthrough
// drafts do not count: the gate must verify what is actually on main.
func committedFiles(root, docsDir string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z", "--", docsDir)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files %s: %w", docsDir, err)
	}
	files := make([]string, 0)
	for path := range strings.SplitSeq(string(out), "\x00") {
		if path != "" {
			files = append(files, filepath.ToSlash(path))
		}
	}
	return files, nil
}
