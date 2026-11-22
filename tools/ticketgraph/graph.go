package main

import (
	"fmt"
	"maps"
	"sort"
)

// Graph is the computed dependency view of all tickets in one feature.
type Graph struct {
	Dir          string
	Tickets      []*Ticket
	ByID         map[string]*Ticket
	State        map[string]State
	Depth        map[string]int
	Unlocks      map[string][]string
	Impact       map[string]int
	Waves        [][]string
	CriticalPath []string
}

func NewGraph(tickets []*Ticket, byID map[string]*Ticket, dir string) (*Graph, error) {
	sort.Slice(tickets, func(i, j int) bool { return idLess(tickets[i].ID, tickets[j].ID) })
	g := &Graph{
		Dir:     dir,
		Tickets: tickets,
		ByID:    byID,
		State:   make(map[string]State, len(tickets)),
		Depth:   make(map[string]int, len(tickets)),
		Unlocks: make(map[string][]string, len(tickets)),
		Impact:  make(map[string]int, len(tickets)),
	}
	if err := g.computeDepth(); err != nil {
		return nil, err
	}
	g.computeStates()
	g.computeUnlocks()
	g.computeImpact()
	g.computeWaves()
	g.computeCriticalPath()
	return g, nil
}

const (
	visitNone = iota
	visitActive
	visitDone
)

func (g *Graph) computeDepth() error {
	color := make(map[string]int, len(g.Tickets))
	var visit func(id string) (int, error)
	visit = func(id string) (int, error) {
		switch color[id] {
		case visitDone:
			return g.Depth[id], nil
		case visitActive:
			return 0, fmt.Errorf("dependency cycle involving %s", id)
		}
		color[id] = visitActive
		maxDep := -1
		for _, dep := range g.ByID[id].BlockedBy {
			if g.ByID[dep] == nil {
				continue
			}
			d, err := visit(dep)
			if err != nil {
				return 0, err
			}
			if d > maxDep {
				maxDep = d
			}
		}
		depth := 0
		if maxDep >= 0 {
			depth = maxDep + 1
		}
		g.Depth[id] = depth
		color[id] = visitDone
		return depth, nil
	}
	for _, t := range g.Tickets {
		if _, err := visit(t.ID); err != nil {
			return err
		}
	}
	return nil
}

func (g *Graph) computeStates() {
	for _, t := range g.Tickets {
		switch t.Status {
		case StatusDone:
			g.State[t.ID] = StateDone
		case StatusInReview:
			g.State[t.ID] = StateInReview
		case StatusOnHold:
			g.State[t.ID] = StateOnHold
		case StatusNeedsInfo:
			g.State[t.ID] = StateNeedsInfo
		default:
			ready := len(t.External) == 0
			for _, dep := range t.BlockedBy {
				if g.State[dep] != StateDone {
					ready = false
					break
				}
			}
			if ready {
				g.State[t.ID] = StateReady
			} else {
				g.State[t.ID] = StateBlocked
			}
		}
	}
}

func (g *Graph) computeUnlocks() {
	for _, t := range g.Tickets {
		for _, dep := range t.BlockedBy {
			g.Unlocks[dep] = append(g.Unlocks[dep], t.ID)
		}
	}
	for id := range g.Unlocks {
		sortIDs(g.Unlocks[id])
	}
}

func (g *Graph) reachable(id string) map[string]bool {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(x string) {
		for _, next := range g.Unlocks[x] {
			if seen[next] {
				continue
			}
			seen[next] = true
			walk(next)
		}
	}
	walk(id)
	return seen
}

func (g *Graph) computeImpact() {
	for _, t := range g.Tickets {
		g.Impact[t.ID] = len(g.reachable(t.ID))
	}
}

func (g *Graph) computeWaves() {
	maxDepth := 0
	for _, t := range g.Tickets {
		if g.Depth[t.ID] > maxDepth {
			maxDepth = g.Depth[t.ID]
		}
	}
	g.Waves = make([][]string, 0, maxDepth+1)
	for depth := 0; depth <= maxDepth; depth++ {
		var wave []string
		for _, t := range g.Tickets {
			if g.Depth[t.ID] == depth {
				wave = append(wave, t.ID)
			}
		}
		sortIDs(wave)
		g.Waves = append(g.Waves, wave)
	}
}

func (g *Graph) computeCriticalPath() {
	end := ""
	for _, t := range g.Tickets {
		switch {
		case end == "":
			end = t.ID
		case g.Depth[t.ID] > g.Depth[end]:
			end = t.ID
		case g.Depth[t.ID] == g.Depth[end] && idLess(end, t.ID):
			end = t.ID
		}
	}
	if end == "" {
		return
	}
	path := []string{end}
	cur := end
	for g.Depth[cur] > 0 {
		best := ""
		for _, dep := range g.ByID[cur].BlockedBy {
			if g.ByID[dep] == nil {
				continue
			}
			switch {
			case best == "":
				best = dep
			case g.Depth[dep] > g.Depth[best]:
				best = dep
			case g.Depth[dep] == g.Depth[best] && idLess(dep, best):
				best = dep
			}
		}
		if best == "" {
			break
		}
		cur = best
		path = append(path, cur)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	g.CriticalPath = path
}

// ReadyIDs returns ready-for-agent tickets whose dependencies are all done,
// ordered by transitive impact (descending) and then ID.
func (g *Graph) ReadyIDs() []string {
	var ids []string
	for _, t := range g.Tickets {
		if g.State[t.ID] == StateReady {
			ids = append(ids, t.ID)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		if g.Impact[ids[i]] != g.Impact[ids[j]] {
			return g.Impact[ids[i]] > g.Impact[ids[j]]
		}
		return idLess(ids[i], ids[j])
	})
	return ids
}

func (g *Graph) stateSnapshot() map[string]State {
	snapshot := make(map[string]State, len(g.State))
	maps.Copy(snapshot, g.State)
	return snapshot
}

func (g *Graph) newlyReady(before map[string]State) []string {
	var ids []string
	for _, t := range g.Tickets {
		if g.State[t.ID] == StateReady && before[t.ID] != StateReady {
			ids = append(ids, t.ID)
		}
	}
	sortIDs(ids)
	return ids
}
