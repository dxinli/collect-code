package config

import (
	"fmt"
	"sort"
	"strings"
)

// Diagnostic is a structured configuration problem. 01a fills File/Line/Col
// and Message; 01b adds RuleID and column-accurate rendering.
type Diagnostic struct {
	File    string
	Line    int
	Col     int
	RuleID  string
	Message string
}

func (d Diagnostic) Error() string {
	var b strings.Builder
	if d.File != "" {
		b.WriteString(d.File)
	}
	if d.Line > 0 {
		if b.Len() > 0 {
			b.WriteByte(':')
		}
		fmt.Fprintf(&b, "%d", d.Line)
	}
	if d.Col > 0 {
		if b.Len() > 0 {
			b.WriteByte(':')
		}
		fmt.Fprintf(&b, "%d", d.Col)
	}
	if b.Len() > 0 {
		b.WriteString(": ")
	}
	if d.RuleID != "" {
		b.WriteString(d.RuleID)
		b.WriteString(": ")
	}
	b.WriteString(d.Message)
	return b.String()
}

// Diagnostics is an ordered collection of problems.
type Diagnostics []Diagnostic

func (ds Diagnostics) Error() string {
	if len(ds) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ds))
	for _, d := range ds {
		parts = append(parts, d.Error())
	}
	return strings.Join(parts, "\n")
}

// fileRank gives the fixed layer order config -> links -> devices -> profiles.
func fileRank(file string) int {
	switch file {
	case "config.yaml":
		return 0
	case "links.yaml":
		return 1
	case "devices.yaml":
		return 2
	}
	if file == "profiles" || strings.HasPrefix(file, "profiles/") {
		return 3
	}
	return 4
}

// sortDiagnostics makes output reproducible: layer, path, line, column, rule
// number, then message.
func sortDiagnostics(ds Diagnostics) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		if ra, rb := fileRank(a.File), fileRank(b.File); ra != rb {
			return ra < rb
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		if ra, rb := ruleNumber(a.RuleID), ruleNumber(b.RuleID); ra != rb {
			return ra < rb
		}
		return a.Message < b.Message
	})
}

func dedupeDiagnostics(ds Diagnostics) Diagnostics {
	if len(ds) < 2 {
		return ds
	}
	out := ds[:1]
	for _, d := range ds[1:] {
		last := out[len(out)-1]
		if d.File == last.File && d.Line == last.Line && d.Col == last.Col &&
			d.RuleID == last.RuleID && d.Message == last.Message {
			continue
		}
		out = append(out, d)
	}
	return out
}

func finalizeDiagnostics(ds Diagnostics) Diagnostics {
	if len(ds) == 0 {
		return nil
	}
	sortDiagnostics(ds)
	return dedupeDiagnostics(ds)
}
