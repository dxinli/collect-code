package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestOnlyPointersAreErrors(t *testing.T) {
	var _ error = (*Diagnostic)(nil)
	var _ error = (*Diagnostics)(nil)
	if _, ok := any(Diagnostic{}).(error); ok {
		t.Fatal("Diagnostic value must not implement error; use *Diagnostic")
	}
	if _, ok := any(Diagnostics{}).(error); ok {
		t.Fatal("Diagnostics value must not implement error; use *Diagnostics")
	}
}

func TestDiagnosticStringStillHumanReadable(t *testing.T) {
	d := Diagnostic{File: "config.yaml", Line: 2, Col: 3, RuleID: RuleQoS, Message: "qos"}
	if got := fmt.Sprint(d); got != "config.yaml:2:3: V45: qos" {
		t.Fatalf("Diagnostic String = %q", got)
	}
	ds := Diagnostics{d}
	if got := fmt.Sprint(ds); got != "config.yaml:2:3: V45: qos" {
		t.Fatalf("Diagnostics String = %q", got)
	}
	if got := (&ds).Error(); got != "config.yaml:2:3: V45: qos" {
		t.Fatalf("Diagnostics Error = %q", got)
	}
}

func TestDiagnosticOrdering(t *testing.T) {
	ds := Diagnostics{
		{File: "profiles/z.yaml", Line: 2, Col: 1, RuleID: RuleScale, Message: "z"},
		{File: "devices.yaml", Line: 9, Col: 5, RuleID: RuleLinkRef, Message: "d"},
		{File: "config.yaml", Line: 8, Col: 5, RuleID: RuleQoS, Message: "q"},
		{File: "links.yaml", Line: 3, Col: 5, RuleID: RuleDriver, Message: "l"},
		{File: "profiles/a.yaml", Line: 10, Col: 1, RuleID: RuleRequiredProfile, Message: "p"},
		{File: "config.yaml", Line: 8, Col: 5, RuleID: RuleLogLevel, Message: "a"},
		{File: "config.yaml", Line: 3, Col: 1, RuleID: RuleRequiredConfig, Message: "m"},
		{File: "config.yaml", Line: 8, Col: 5, RuleID: RuleLogLevel, Message: "b"},
	}
	got := finalizeDiagnostics(ds)
	want := []string{
		"config.yaml:3:1",
		"config.yaml:8:5",
		"config.yaml:8:5",
		"config.yaml:8:5",
		"links.yaml:3:5",
		"devices.yaml:9:5",
		"profiles/a.yaml:10:1",
		"profiles/z.yaml:2:1",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d diagnostics, want %d: %v", len(got), len(want), got)
	}
	for i, prefix := range want {
		if !strings.HasPrefix(got[i].Error(), prefix+":") {
			t.Errorf("diagnostic %d = %q, want prefix %q", i, got[i].Error(), prefix)
		}
	}
	if got[1].RuleID != RuleQoS || got[2].RuleID != RuleLogLevel || got[3].Message != "b" {
		t.Fatalf("same-position ordering wrong: %v", got[1:4])
	}
}

func TestDedupeDiagnostics(t *testing.T) {
	d := Diagnostic{File: "config.yaml", Line: 1, Col: 1, RuleID: RuleSyntax, Message: "same"}
	ds := finalizeDiagnostics(Diagnostics{d, d, {File: "config.yaml", Line: 1, Col: 1, RuleID: RuleSyntax, Message: "other"}})
	if len(ds) != 2 {
		t.Fatalf("dedupe = %v", ds)
	}
}
