package main

import (
	"sort"
	"strconv"
)

// Status is the triage state recorded in an issue file's **Status:** line.
type Status string

const (
	StatusDone          Status = "done"
	StatusInReview      Status = "in-review"
	StatusReadyForAgent Status = "ready-for-agent"
	StatusOnHold        Status = "on-hold"
	StatusNeedsInfo     Status = "needs-info"
	StatusBacklog       Status = "backlog"
)

var knownStatuses = map[Status]bool{
	StatusDone:          true,
	StatusInReview:      true,
	StatusReadyForAgent: true,
	StatusOnHold:        true,
	StatusNeedsInfo:     true,
	StatusBacklog:       true,
}

// State is the dependency state derived from Status + Blocked by.
type State string

const (
	StateDone      State = "done"
	StateInReview  State = "in-review"
	StateReady     State = "ready"
	StateBlocked   State = "blocked"
	StateOnHold    State = "on-hold"
	StateNeedsInfo State = "needs-info"
)

func (s State) Label() string {
	switch s {
	case StateDone:
		return "done"
	case StateInReview:
		return "in review"
	case StateReady:
		return "ready now"
	case StateBlocked:
		return "blocked"
	case StateOnHold:
		return "on-hold"
	case StateNeedsInfo:
		return "needs-info"
	default:
		return string(s)
	}
}

// Ticket is one parsed issue file.
type Ticket struct {
	ID         string
	Title      string
	What       string
	Note       string
	Status     Status
	BlockedRaw string
	BlockedBy  []string
	External   []string
	Path       string
}

// idLess orders 01a, 01b, 02a, ..., 10, 12a, 20 naturally.
func idLess(a, b string) bool {
	na, sa := splitID(a)
	nb, sb := splitID(b)
	if na != nb {
		return na < nb
	}
	return sa < sb
}

func splitID(id string) (int, string) {
	i := 0
	for i < len(id) && id[i] >= '0' && id[i] <= '9' {
		i++
	}
	n, _ := strconv.Atoi(id[:i])
	return n, id[i:]
}

func sortIDs(ids []string) {
	sort.Slice(ids, func(i, j int) bool { return idLess(ids[i], ids[j]) })
}
