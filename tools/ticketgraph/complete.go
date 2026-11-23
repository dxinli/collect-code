package main

import (
	"fmt"
	"io"
	"strings"
)

type skipInfo struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type completeOutput struct {
	Updated    []string   `json:"updated"`
	Skipped    []skipInfo `json:"skipped"`
	NewlyReady []string   `json:"newly_ready"`
	PR         int        `json:"pr,omitempty"`
}

func completeTickets(dir string, ids []string, pr int) (completeOutput, error) {
	g, err := LoadGraph(dir)
	if err != nil {
		return completeOutput{}, err
	}
	before := g.stateSnapshot()
	out := completeOutput{
		Updated:    []string{},
		Skipped:    []skipInfo{},
		NewlyReady: []string{},
		PR:         pr,
	}
	for _, id := range ids {
		t := g.ByID[id]
		if t == nil {
			return completeOutput{}, fmt.Errorf("unknown ticket %q", id)
		}
		if t.IsMerged() {
			out.Skipped = append(out.Skipped, skipInfo{ID: id, Reason: "already merged"})
			continue
		}
		mergedValue := "manual"
		if pr > 0 {
			mergedValue = fmt.Sprintf("#%d", pr)
		}
		if err := setMerged(t.Path, mergedValue); err != nil {
			return completeOutput{}, err
		}
		out.Updated = append(out.Updated, id)
	}
	after, err := LoadGraph(dir)
	if err != nil {
		return completeOutput{}, err
	}
	out.NewlyReady = after.newlyReady(before)
	if out.NewlyReady == nil {
		out.NewlyReady = []string{}
	}
	sortIDs(out.Updated)
	sortIDs(out.NewlyReady)
	return out, nil
}

func parseTicketList(value string) []string {
	var ids []string
	for part := range strings.SplitSeq(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			ids = append(ids, part)
		}
	}
	return uniqueIDs(ids)
}

func printCompleteSummary(w io.Writer, out completeOutput) {
	if len(out.Updated) > 0 {
		_, _ = fmt.Fprintf(w, "updated: %s\n", strings.Join(out.Updated, ", "))
	}
	for _, s := range out.Skipped {
		_, _ = fmt.Fprintf(w, "skipped %s: %s\n", s.ID, s.Reason)
	}
	if len(out.NewlyReady) > 0 {
		_, _ = fmt.Fprintf(w, "newly ready: %s\n", strings.Join(out.NewlyReady, ", "))
	}
	if len(out.Updated) == 0 && len(out.Skipped) == 0 {
		_, _ = fmt.Fprintln(w, "nothing to do")
	}
}
