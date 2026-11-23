package main

import (
	"sort"
	"strconv"
)

// Status is the triage state recorded in an issue file's **Status:** line.
// These are the five mattpocock triage labels.
type Status string

const (
	StatusNeedsTriage   Status = "needs-triage"
	StatusNeedsInfo     Status = "needs-info"
	StatusReadyForAgent Status = "ready-for-agent"
	StatusReadyForHuman Status = "ready-for-human"
	StatusWontFix       Status = "wontfix"
)

var knownStatuses = map[Status]bool{
	StatusNeedsTriage:   true,
	StatusNeedsInfo:     true,
	StatusReadyForAgent: true,
	StatusReadyForHuman: true,
	StatusWontFix:       true,
}

// State is the dependency state derived from Status + PR/Merged fields +
// Blocked by. Only the first two are lifecycle states; the rest are triage
// or dependency states.
type State string

const (
	StateDone          State = "done"
	StateInReview      State = "in-review"
	StateReady         State = "ready"
	StateBlocked       State = "blocked"
	StateNeedsTriage   State = "needs-triage"
	StateNeedsInfo     State = "needs-info"
	StateReadyForHuman State = "ready-for-human"
	StateWontFix       State = "wontfix"
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
	case StateNeedsTriage:
		return "needs-triage"
	case StateNeedsInfo:
		return "needs-info"
	case StateReadyForHuman:
		return "ready-for-human"
	case StateWontFix:
		return "wontfix"
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
	PR         string
	Merged     string
	BlockedRaw string
	BlockedBy  []string
	External   []string
	Path       string
}

// IsMerged reports whether the post-merge bot has recorded this ticket.
func (t *Ticket) IsMerged() bool { return t.Merged != "" }

// IsInReview reports whether a PR is open and not yet merged.
func (t *Ticket) IsInReview() bool { return t.Merged == "" && t.PR != "" }

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
