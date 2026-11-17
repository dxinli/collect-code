package config

import (
	"fmt"
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
