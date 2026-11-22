package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "render":
		err = cmdRender(os.Args[2:])
	case "set-status":
		err = cmdSetStatus(os.Args[2:])
	case "complete":
		err = cmdComplete(os.Args[2:])
	case "ready":
		err = cmdReady(os.Args[2:])
	case "resolve":
		err = cmdResolve(os.Args[2:])
	case "guard":
		err = cmdGuard(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "ticketgraph:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: ticketgraph <command> [flags]

commands:
  render      generate <dir>/ticket-dependency.html
  set-status  rewrite one **Status:** line
  complete    mark tickets done and report newly ready tickets
  ready       list tickets whose dependencies are done
  resolve     resolve ticket IDs from a PR body/labels/branch or event
  guard       enforce "feat/* cannot write done"

common flags:
  --dir .scratch/collector   feature directory
  --feature collector        shorthand for --dir .scratch/<feature>`)
}

func resolveDir(dir, feature string) string {
	if feature != "" {
		return filepath.Join(".scratch", feature)
	}
	return dir
}

func cmdRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	dir := fs.String("dir", ".scratch/collector", "feature directory")
	feature := fs.String("feature", "", "shorthand for --dir .scratch/<feature>")
	check := fs.Bool("check", false, "verify the committed HTML is up to date")
	if err := fs.Parse(args); err != nil {
		return err
	}
	d := resolveDir(*dir, *feature)
	g, err := LoadGraph(d)
	if err != nil {
		return err
	}
	if *check {
		return checkRender(g)
	}
	if err := writeRender(g); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", renderPath(d))
	return nil
}

func cmdSetStatus(args []string) error {
	fs := flag.NewFlagSet("set-status", flag.ContinueOnError)
	dir := fs.String("dir", ".scratch/collector", "feature directory")
	feature := fs.String("feature", "", "shorthand for --dir .scratch/<feature>")
	ticket := fs.String("ticket", "", "ticket ID, e.g. 02b")
	status := fs.String("status", "", "new status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ticket == "" || *status == "" {
		return errors.New("set-status requires --ticket and --status")
	}
	d := resolveDir(*dir, *feature)
	g, err := LoadGraph(d)
	if err != nil {
		return err
	}
	t := g.ByID[*ticket]
	if t == nil {
		return fmt.Errorf("unknown ticket %q", *ticket)
	}
	if err := setStatus(t.Path, Status(*status)); err != nil {
		return err
	}
	fmt.Printf("%s: %s -> %s\n", t.ID, t.Status, *status)
	return nil
}

func cmdComplete(args []string) error {
	fs := flag.NewFlagSet("complete", flag.ContinueOnError)
	dir := fs.String("dir", ".scratch/collector", "feature directory")
	feature := fs.String("feature", "", "shorthand for --dir .scratch/<feature>")
	ticket := fs.String("ticket", "", "comma-separated ticket IDs")
	pr := fs.Int("pr", 0, "pull request number for the report")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ticket == "" {
		return errors.New("complete requires --ticket")
	}
	out, err := completeTickets(resolveDir(*dir, *feature), parseTicketList(*ticket), *pr)
	if err != nil {
		return err
	}
	if *jsonOut {
		printCompleteSummary(os.Stderr, out)
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	printCompleteSummary(os.Stdout, out)
	return nil
}

func cmdReady(args []string) error {
	fs := flag.NewFlagSet("ready", flag.ContinueOnError)
	dir := fs.String("dir", ".scratch/collector", "feature directory")
	feature := fs.String("feature", "", "shorthand for --dir .scratch/<feature>")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	g, err := LoadGraph(resolveDir(*dir, *feature))
	if err != nil {
		return err
	}
	type readyItem struct {
		ID      string   `json:"id"`
		Title   string   `json:"title"`
		Unlocks []string `json:"unlocks"`
		Impact  int      `json:"impact"`
	}
	var ready []readyItem
	var blocked []string
	for _, id := range g.ReadyIDs() {
		unlocks := g.Unlocks[id]
		if unlocks == nil {
			unlocks = []string{}
		}
		ready = append(ready, readyItem{ID: id, Title: g.ByID[id].Title, Unlocks: unlocks, Impact: g.Impact[id]})
	}
	for _, t := range g.Tickets {
		if g.State[t.ID] == StateBlocked {
			blocked = append(blocked, t.ID)
		}
	}
	if *jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"ready": ready, "blocked": blocked})
	}
	if len(ready) == 0 {
		fmt.Println("no ready tickets")
		return nil
	}
	for _, item := range ready {
		fmt.Printf("%s\t%s\n", item.ID, item.Title)
	}
	return nil
}

func cmdResolve(args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	dir := fs.String("dir", ".scratch/collector", "feature directory")
	feature := fs.String("feature", "", "shorthand for --dir .scratch/<feature>")
	event := fs.String("event", "", "GitHub pull_request event JSON")
	branch := fs.String("branch", "", "PR head branch")
	body := fs.String("body", "", "PR body")
	bodyFile := fs.String("body-file", "", "file containing the PR body")
	labels := fs.String("labels", "", "comma-separated PR labels")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	g, err := LoadGraph(resolveDir(*dir, *feature))
	if err != nil {
		return err
	}
	var info PRInfo
	if *event != "" {
		var ok bool
		info, ok, err = readPREvent(*event)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s is not a pull_request event", *event)
		}
	} else {
		info = PRInfo{Branch: *branch, Body: *body}
		if *bodyFile != "" {
			data, err := os.ReadFile(*bodyFile)
			if err != nil {
				return err
			}
			info.Body = string(data)
		}
		if *labels != "" {
			info.Labels = strings.Split(*labels, ",")
		}
	}
	ids, source, err := resolveTickets(g, info)
	if err != nil {
		return err
	}
	if *jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"tickets": ids, "source": source})
	}
	fmt.Println(strings.Join(ids, ","))
	return nil
}

func cmdGuard(args []string) error {
	fs := flag.NewFlagSet("guard", flag.ContinueOnError)
	dir := fs.String("dir", ".scratch/collector", "feature directory")
	feature := fs.String("feature", "", "shorthand for --dir .scratch/<feature>")
	event := fs.String("event", "", "GitHub pull_request event JSON (defaults to GITHUB_EVENT_PATH)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	eventPath := *event
	if eventPath == "" {
		eventPath = os.Getenv("GITHUB_EVENT_PATH")
	}
	if eventPath == "" {
		fmt.Println("ticketgraph: guard skipped (no GITHUB_EVENT_PATH)")
		return nil
	}
	info, ok, err := readPREvent(eventPath)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("ticketgraph: guard skipped (not a pull_request event)")
		return nil
	}
	g, err := LoadGraph(resolveDir(*dir, *feature))
	if err != nil {
		return err
	}
	ids, source, err := resolveTickets(g, info)
	if err != nil {
		return err
	}
	if err := guardGraph(g, info.Branch, ids); err != nil {
		return err
	}
	if len(ids) == 0 {
		fmt.Println("ticketgraph: guard skipped (no ticket mapping)")
		return nil
	}
	fmt.Printf("ticketgraph: guard OK (%s: %s)\n", source, strings.Join(ids, ", "))
	return nil
}
