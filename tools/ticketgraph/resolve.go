package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	ticketLineRe   = regexp.MustCompile(`(?mi)^[ \t]*[-*]?[ \t]*\*{0,2}(tickets?)\*{0,2}[ \t]*:[ \t]*(.+)$`)
	labelTicketRe  = regexp.MustCompile(`(?i)^tickets?[:\s]+(.+)$`)
	branchTicketRe = regexp.MustCompile(`^feat/([0-9]{2}[a-z]?)-`)
	botBranchRe    = regexp.MustCompile(`^chore/tickets-sync-`)
	htmlCommentRe  = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// PRInfo is the subset of a pull request needed to find its ticket(s).
type PRInfo struct {
	Body   string
	Branch string
	Labels []string
	Number int
	Merged bool
}

// resolveTickets returns the ticket IDs declared by a PR, the source of the
// declaration, and an error for an explicit but unknown ticket.
//
// Priority: PR body (Ticket: / Tickets:) > label (ticket:02b) > branch name.
func resolveTickets(g *Graph, info PRInfo) ([]string, string, error) {
	for line := range strings.SplitSeq(info.Body, "\n") {
		m := ticketLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ids, err := parseTicketIDs(m[2], g)
		if err != nil {
			return nil, "body", err
		}
		if len(ids) > 0 {
			return ids, "body", nil
		}
	}
	for _, label := range info.Labels {
		m := labelTicketRe.FindStringSubmatch(label)
		if m == nil {
			continue
		}
		ids, err := parseTicketIDs(m[1], g)
		if err != nil {
			return nil, "label", err
		}
		if len(ids) > 0 {
			return ids, "label", nil
		}
	}
	if m := branchTicketRe.FindStringSubmatch(info.Branch); m != nil {
		if g.ByID[m[1]] == nil {
			return nil, "branch", fmt.Errorf("branch names unknown ticket %q", m[1])
		}
		return []string{m[1]}, "branch", nil
	}
	return nil, "", nil
}

func parseTicketIDs(value string, g *Graph) ([]string, error) {
	value = htmlCommentRe.ReplaceAllString(value, "")
	seen := map[string]bool{}
	var ids []string
	for _, id := range ticketIDRe.FindAllString(value, -1) {
		if g.ByID[id] == nil {
			return nil, fmt.Errorf("unknown ticket %q in %q", id, strings.TrimSpace(value))
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sortIDs(ids)
	return ids, nil
}

type pullRequestEvent struct {
	PullRequest *struct {
		Body   string `json:"body"`
		Number int    `json:"number"`
		Merged bool   `json:"merged"`
		Head   struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	} `json:"pull_request"`
}

func readPREvent(path string) (PRInfo, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PRInfo{}, false, err
	}
	var event pullRequestEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return PRInfo{}, false, err
	}
	if event.PullRequest == nil {
		return PRInfo{}, false, nil
	}
	info := PRInfo{
		Body:   event.PullRequest.Body,
		Branch: event.PullRequest.Head.Ref,
		Number: event.PullRequest.Number,
		Merged: event.PullRequest.Merged,
	}
	for _, label := range event.PullRequest.Labels {
		info.Labels = append(info.Labels, label.Name)
	}
	return info, true, nil
}

// guardGraph enforces the two-phase status model on a PR.
func guardGraph(g *Graph, branch string, tickets []string) error {
	if botBranchRe.MatchString(branch) {
		return nil
	}
	if len(tickets) == 0 {
		if strings.HasPrefix(branch, "feat/") {
			return fmt.Errorf("feat/* PR must declare its ticket with '**Ticket:**' or a ticket:<id> label")
		}
		return nil
	}
	for _, id := range tickets {
		t := g.ByID[id]
		if t == nil {
			return fmt.Errorf("unknown ticket %q", id)
		}
		switch t.Status {
		case StatusDone:
			return fmt.Errorf("%s is already done; done can only be written after merge by the tickets-sync bot", id)
		case StatusOnHold, StatusNeedsInfo:
			fmt.Fprintf(os.Stderr, "ticketgraph: warning: %s is %s; delivery still needs a human decision\n", id, t.Status)
		}
	}
	return nil
}
