package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	titleRe    = regexp.MustCompile(`(?m)^#\s+([0-9]{2}[a-z]?)\s*:\s*(.+)$`)
	statusRe   = regexp.MustCompile(`(?m)^\*\*Status:\*\*[ \t]*(.*)$`)
	prRe       = regexp.MustCompile(`(?m)^\*\*PR:\*\*[ \t]*(.*)$`)
	mergedRe   = regexp.MustCompile(`(?m)^\*\*Merged:\*\*[ \t]*(.*)$`)
	blockedRe  = regexp.MustCompile(`(?m)^\*\*Blocked by:\*\*[ \t]*(.*)$`)
	whatRe     = regexp.MustCompile(`(?m)^\*\*What to build:\*\*[ \t]*(.*)$`)
	noteRe     = regexp.MustCompile(`(?m)^\*\*Note:\*\*[ \t]*(.*)$`)
	ticketIDRe = regexp.MustCompile(`\b([0-9]{2}[a-z]?)\b`)
)

func parseIssue(path string) (*Ticket, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)
	m := titleRe.FindStringSubmatch(text)
	if m == nil {
		return nil, fmt.Errorf("%s: missing '# <id>: <title>' heading", path)
	}
	t := &Ticket{
		ID:    m[1],
		Title: strings.TrimSpace(m[2]),
		Path:  path,
	}
	if m := whatRe.FindStringSubmatch(text); m != nil {
		t.What = strings.TrimSpace(m[1])
	}
	if m := noteRe.FindStringSubmatch(text); m != nil {
		t.Note = strings.TrimSpace(m[1])
	}
	sm := statusRe.FindStringSubmatch(text)
	if sm == nil {
		return nil, fmt.Errorf("%s: missing '**Status:**' line", path)
	}
	t.Status = Status(strings.TrimSpace(sm[1]))
	if !knownStatuses[t.Status] {
		return nil, fmt.Errorf("%s: unknown status %q (expected one of the five triage labels)", path, t.Status)
	}
	if m := prRe.FindStringSubmatch(text); m != nil {
		t.PR = strings.TrimSpace(m[1])
	}
	if m := mergedRe.FindStringSubmatch(text); m != nil {
		t.Merged = strings.TrimSpace(m[1])
	}
	if bm := blockedRe.FindStringSubmatch(text); bm != nil {
		t.BlockedRaw = strings.TrimSpace(bm[1])
		if !strings.HasPrefix(strings.ToLower(t.BlockedRaw), "none") {
			t.BlockedBy = uniqueIDs(ticketIDRe.FindAllString(t.BlockedRaw, -1))
		}
	}
	return t, nil
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sortIDs(out)
	return out
}

// LoadGraph reads <dir>/issues/*.md and computes the dependency graph.
func LoadGraph(dir string) (*Graph, error) {
	issuesDir := filepath.Join(dir, "issues")
	paths, err := filepath.Glob(filepath.Join(issuesDir, "*.md"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no issue files in %s", issuesDir)
	}
	sort.Strings(paths)

	tickets := make([]*Ticket, 0, len(paths))
	byID := make(map[string]*Ticket, len(paths))
	for _, path := range paths {
		t, err := parseIssue(path)
		if err != nil {
			return nil, err
		}
		if prev, ok := byID[t.ID]; ok {
			return nil, fmt.Errorf("duplicate ticket %s in %s and %s", t.ID, prev.Path, t.Path)
		}
		byID[t.ID] = t
		tickets = append(tickets, t)
	}

	for _, t := range tickets {
		known := t.BlockedBy[:0]
		for _, id := range t.BlockedBy {
			if byID[id] == nil {
				t.External = append(t.External, id)
				continue
			}
			known = append(known, id)
		}
		t.BlockedBy = known
	}

	return NewGraph(tickets, byID, dir)
}

// setStatus rewrites exactly one **Status:** line in an issue file.
func setStatus(path string, status Status) error {
	if !knownStatuses[status] {
		return fmt.Errorf("unknown status %q", status)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	matches := statusRe.FindAllIndex(data, -1)
	if len(matches) != 1 {
		return fmt.Errorf("%s: expected one **Status:** line, found %d", path, len(matches))
	}
	loc := matches[0]
	replaced := make([]byte, 0, len(data)+len(status))
	replaced = append(replaced, data[:loc[0]]...)
	replaced = append(replaced, "**Status:** "...)
	replaced = append(replaced, status...)
	replaced = append(replaced, data[loc[1]:]...)
	return os.WriteFile(path, replaced, 0o644)
}

// setField replaces a **Label:** line, or inserts it after the Status line.
func setField(path, label, value string) error {
	re := regexp.MustCompile(`(?m)^\*\*` + regexp.QuoteMeta(label) + `:\*\*[ \t]*(.*)$`)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	matches := re.FindAllIndex(data, -1)
	var out []byte
	switch len(matches) {
	case 1:
		loc := matches[0]
		out = append(out, data[:loc[0]]...)
		out = append(out, "**"+label+":** "+value...)
		out = append(out, data[loc[1]:]...)
	case 0:
		sm := statusRe.FindIndex(data)
		if sm == nil {
			return fmt.Errorf("%s: missing '**Status:**' line", path)
		}
		out = append(out, data[:sm[1]]...)
		out = append(out, '\n', '\n')
		out = append(out, "**"+label+":** "+value...)
		out = append(out, data[sm[1]:]...)
	default:
		return fmt.Errorf("%s: expected at most one **%s:** line, found %d", path, label, len(matches))
	}
	return os.WriteFile(path, out, 0o644)
}

func setPR(path, value string) error     { return setField(path, "PR", value) }
func setMerged(path, value string) error { return setField(path, "Merged", value) }
