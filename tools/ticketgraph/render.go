package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

//go:embed template.html
var templateHTML string

var ticketTemplate = template.Must(template.New("ticket-dependency").Parse(templateHTML))

type cardData struct {
	ID         string
	Title      string
	State      string
	StateLabel string
	BlockedBy  string
	Unlocks    string
	Summary    string
	Impact     int
}

type nodeData struct {
	ID    string
	Title string
	State string
}

type waveData struct {
	Index int
	Count int
	Cards []cardData
}

type rowData struct {
	ID         string
	Title      string
	BlockedBy  string
	State      string
	StateLabel string
	Unlocks    string
}

type renderData struct {
	PageTitle        string
	Eyebrow          string
	Feature          string
	Source           string
	Total            int
	Done             int
	InReview         int
	Ready            int
	Blocked          int
	NeedsTriage      int
	NeedsInfo        int
	ReadyForHuman    int
	WontFix          int
	ReadyCards       []cardData
	CriticalPath     []nodeData
	CriticalPathNote string
	Waves            []waveData
	Rows             []rowData
}

func buildRenderData(g *Graph) renderData {
	base := filepath.Base(filepath.Clean(g.Dir))
	data := renderData{
		PageTitle: base + " Ticket 依赖",
		Eyebrow:   base + " · 实施路线",
		Feature:   base,
		Source:    filepath.ToSlash(filepath.Join(g.Dir, "issues", "*.md")),
		Total:     len(g.Tickets),
	}
	if base == "collector" {
		data.PageTitle = "采集引擎 Ticket 依赖"
		data.Eyebrow = "采集引擎 · 实施路线"
		data.Feature = "采集引擎"
	}

	for _, t := range g.Tickets {
		switch g.State[t.ID] {
		case StateDone:
			data.Done++
		case StateInReview:
			data.InReview++
		case StateReady:
			data.Ready++
		case StateBlocked:
			data.Blocked++
		case StateNeedsTriage:
			data.NeedsTriage++
		case StateNeedsInfo:
			data.NeedsInfo++
		case StateReadyForHuman:
			data.ReadyForHuman++
		case StateWontFix:
			data.WontFix++
		}
	}

	for _, id := range g.ReadyIDs() {
		data.ReadyCards = append(data.ReadyCards, makeCard(g, g.ByID[id], true))
	}
	for _, id := range g.CriticalPath {
		data.CriticalPath = append(data.CriticalPath, nodeData{
			ID:    id,
			Title: g.ByID[id].Title,
			State: string(g.State[id]),
		})
	}
	data.CriticalPathNote = criticalPathNote(g)
	for i, wave := range g.Waves {
		w := waveData{Index: i, Count: len(wave)}
		for _, id := range wave {
			w.Cards = append(w.Cards, makeCard(g, g.ByID[id], false))
		}
		data.Waves = append(data.Waves, w)
	}
	for _, t := range g.Tickets {
		data.Rows = append(data.Rows, rowData{
			ID:         t.ID,
			Title:      t.Title,
			BlockedBy:  displayBlocked(t),
			State:      string(g.State[t.ID]),
			StateLabel: g.State[t.ID].Label(),
			Unlocks:    displayUnlocks(g, t.ID),
		})
	}
	return data
}

func makeCard(g *Graph, t *Ticket, withSummary bool) cardData {
	unlocks := g.Unlocks[t.ID]
	card := cardData{
		ID:         t.ID,
		Title:      t.Title,
		State:      string(g.State[t.ID]),
		StateLabel: g.State[t.ID].Label(),
		BlockedBy:  displayBlocked(t),
		Unlocks:    displayUnlocks(g, t.ID),
		Impact:     g.Impact[t.ID],
	}
	if withSummary {
		switch {
		case t.Note != "":
			card.Summary = t.Note
		case len(unlocks) == 0:
			card.Summary = "依赖已全部完成；没有直接下游 ticket。"
		default:
			card.Summary = fmt.Sprintf("依赖已全部完成；直接解锁 %d 个，传递影响 %d 个。", len(unlocks), g.Impact[t.ID])
		}
	}
	return card
}

func displayBlocked(t *Ticket) string {
	if t.BlockedRaw == "" || strings.HasPrefix(strings.ToLower(t.BlockedRaw), "none") {
		return "—"
	}
	return t.BlockedRaw
}

func displayUnlocks(g *Graph, id string) string {
	unlocks := g.Unlocks[id]
	if len(unlocks) == 0 {
		return "—"
	}
	return strings.Join(unlocks, ", ")
}

func criticalPathNote(g *Graph) string {
	if len(g.CriticalPath) == 0 {
		return "没有可计算的路径。"
	}
	first := g.CriticalPath[0]
	last := g.CriticalPath[len(g.CriticalPath)-1]
	note := fmt.Sprintf("从 %s 到 %s 的最长路径，共 %d 个节点。", first, last, len(g.CriticalPath))
	lt := g.ByID[last]
	if lt.Status == StatusNeedsInfo {
		note += " 终点仍需要外部信息。"
	}
	if len(lt.External) > 0 {
		note += " 终点还依赖外部条件。"
	}
	return note
}

func renderGraph(g *Graph) ([]byte, error) {
	var b strings.Builder
	if err := ticketTemplate.Execute(&b, buildRenderData(g)); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func renderPath(dir string) string {
	return filepath.Join(dir, "ticket-dependency.html")
}

func writeRender(g *Graph) error {
	data, err := renderGraph(g)
	if err != nil {
		return err
	}
	return os.WriteFile(renderPath(g.Dir), data, 0o644)
}

func checkRender(g *Graph) error {
	data, err := renderGraph(g)
	if err != nil {
		return err
	}
	path := renderPath(g.Dir)
	existing, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w (run `make tickets`)", path, err)
	}
	if !bytes.Equal(existing, data) {
		return fmt.Errorf("%s is stale; run `make tickets`", path)
	}
	return nil
}
