package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIssue(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, "issues", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureGraph(t *testing.T) (*Graph, string) {
	t.Helper()
	dir := t.TempDir()
	writeIssue(t, dir, "01a-alpha.md", "# 01a: Alpha\n\n**Status:** done\n")
	writeIssue(t, dir, "02a-beta.md", "# 02a: Beta\n\n**Status:** done\n\n**Blocked by:** 01a\n")
	writeIssue(t, dir, "02b-review.md", "# 02b: Gamma\n\n**Status:** in-review\n\n**Blocked by:** 02a\n")
	writeIssue(t, dir, "03a-ready.md", "# 03a: Delta\n\n**Status:** ready-for-agent\n\n**Blocked by:** 02a\n")
	writeIssue(t, dir, "03b-blocked.md", "# 03b: Epsilon\n\n**Status:** ready-for-agent\n\n**Blocked by:** 02a,03a\n")
	writeIssue(t, dir, "04-hold.md", "# 04: Zeta\n\n**Status:** on-hold\n\n**Blocked by:** 02a\n")
	writeIssue(t, dir, "05-info.md", "# 05: Eta\n\n**Status:** needs-info\n\n**Blocked by:** 02a\n")
	writeIssue(t, dir, "06-review-dep.md", "# 06: Theta\n\n**Status:** ready-for-agent\n\n**Blocked by:** 02b\n")
	writeIssue(t, dir, "07-ready.md", "# 07: Iota\n\n**Status:** ready-for-agent\n\n**Blocked by:** 02a\n")
	writeIssue(t, dir, "08-external.md", "# 08: Kappa\n\n**Status:** ready-for-agent\n\n**Blocked by:** 01z\n")
	g, err := LoadGraph(dir)
	if err != nil {
		t.Fatal(err)
	}
	return g, dir
}

func TestParseIssue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01a-x.md")
	content := "# 01a: Title\n\n**What to build:** thing\n\n**Blocked by:** None (can start immediately)\n\n**Status:** ready-for-agent\n\n**Note:** note text\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tk, err := parseIssue(path)
	if err != nil {
		t.Fatal(err)
	}
	if tk.ID != "01a" || tk.Title != "Title" || tk.What != "thing" || tk.Note != "note text" {
		t.Fatalf("unexpected ticket: %+v", tk)
	}
	if tk.Status != StatusReadyForAgent || len(tk.BlockedBy) != 0 {
		t.Fatalf("unexpected status/deps: %+v", tk)
	}
}

func TestParseIssueRejectsUnknownStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01a-x.md")
	if err := os.WriteFile(path, []byte("# 01a: Title\n\n**Status:** nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := parseIssue(path); err == nil {
		t.Fatal("expected unknown status error")
	}
}

func TestGraphStates(t *testing.T) {
	g, _ := fixtureGraph(t)
	want := map[string]State{
		"01a": StateDone,
		"02a": StateDone,
		"02b": StateInReview,
		"03a": StateReady,
		"03b": StateBlocked,
		"04":  StateOnHold,
		"05":  StateNeedsInfo,
		"06":  StateBlocked,
		"07":  StateReady,
		"08":  StateBlocked,
	}
	for id, state := range want {
		if g.State[id] != state {
			t.Errorf("%s: got state %s, want %s", id, g.State[id], state)
		}
	}
	if g.Depth["03b"] != 3 {
		t.Errorf("03b depth = %d, want 3", g.Depth["03b"])
	}
	if got := strings.Join(g.Unlocks["02a"], ","); got != "02b,03a,03b,04,05,07" {
		t.Errorf("02a unlocks = %q", got)
	}
	if got := strings.Join(g.ReadyIDs(), ","); got != "03a,07" {
		t.Errorf("ready = %q, want 03a,07", got)
	}
	if len(g.CriticalPath) < 4 {
		t.Errorf("critical path too short: %v", g.CriticalPath)
	}
}

func TestCompleteTickets(t *testing.T) {
	dir := t.TempDir()
	writeIssue(t, dir, "01a-a.md", "# 01a: A\n\n**Status:** done\n")
	writeIssue(t, dir, "02a-b.md", "# 02a: B\n\n**Status:** ready-for-agent\n\n**Blocked by:** 01a\n")
	writeIssue(t, dir, "02b-c.md", "# 02b: C\n\n**Status:** ready-for-agent\n\n**Blocked by:** 02a\n")
	out, err := completeTickets(dir, []string{"02a"}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out.Updated, ",") != "02a" {
		t.Fatalf("updated = %v", out.Updated)
	}
	if strings.Join(out.NewlyReady, ",") != "02b" {
		t.Fatalf("newly ready = %v", out.NewlyReady)
	}
	g, err := LoadGraph(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.ByID["02a"].Status != StatusDone {
		t.Fatalf("02a status = %s", g.ByID["02a"].Status)
	}
	out, err = completeTickets(dir, []string{"02a", "02b"}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Updated) != 1 || out.Updated[0] != "02b" {
		t.Fatalf("second run updated = %v", out.Updated)
	}
}

func TestResolveTickets(t *testing.T) {
	g, _ := fixtureGraph(t)
	tests := []struct {
		name   string
		info   PRInfo
		want   string
		source string
	}{
		{"body", PRInfo{Body: "- **Ticket:** 02b"}, "02b", "body"},
		{"body-multi", PRInfo{Body: "**Tickets:** 01a, 02a"}, "01a,02a", "body"},
		{"label", PRInfo{Labels: []string{"ticket:03a"}}, "03a", "label"},
		{"template-placeholder", PRInfo{Body: "- **Ticket:** <!-- e.g. 02b -->", Branch: "feat/02b-store"}, "02b", "branch"},
		{"branch-fallback", PRInfo{Body: "REQ-SEAM-02", Branch: "feat/02b-store"}, "02b", "branch"},
		{"none", PRInfo{Branch: "docs/x"}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, source, err := resolveTickets(g, tt.info)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(ids, ","); got != tt.want || source != tt.source {
				t.Fatalf("got %q from %q, want %q from %q", got, source, tt.want, tt.source)
			}
		})
	}
	if _, _, err := resolveTickets(g, PRInfo{Body: "**Ticket:** 99z"}); err == nil {
		t.Fatal("expected unknown ticket error")
	}
}

func TestGuard(t *testing.T) {
	g, _ := fixtureGraph(t)
	if err := guardGraph(g, "feat/02b-store", []string{"02b"}); err != nil {
		t.Fatalf("in-review ticket should pass: %v", err)
	}
	if err := guardGraph(g, "feat/01a-x", []string{"01a"}); err == nil {
		t.Fatal("done ticket should fail")
	}
	if err := guardGraph(g, "chore/tickets-sync-1", []string{"01a"}); err != nil {
		t.Fatalf("bot branch should be skipped: %v", err)
	}
	if err := guardGraph(g, "feat/no-ticket", nil); err == nil {
		t.Fatal("feat PR without a ticket should fail")
	}
	if err := guardGraph(g, "docs/x", nil); err != nil {
		t.Fatalf("non-feat PR without a ticket should pass: %v", err)
	}
}

func TestRenderDeterministic(t *testing.T) {
	g, dir := fixtureGraph(t)
	first, err := renderGraph(g)
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderGraph(g)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("render is not deterministic")
	}
	html := string(first)
	for _, want := range []string{"<!doctype html>", "tools/ticketgraph", "in-review", "02b", "ready now"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
	if err := os.WriteFile(renderPath(dir), first, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkRender(g); err != nil {
		t.Fatalf("check should pass: %v", err)
	}
	if err := os.WriteFile(renderPath(dir), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkRender(g); err == nil {
		t.Fatal("check should fail on stale HTML")
	}
}
