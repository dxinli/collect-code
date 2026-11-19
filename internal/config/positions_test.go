package config

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

func testIndex(t *testing.T, src string) *positionIndex {
	t.Helper()
	var n yaml.Node
	if err := yaml.Load([]byte(src), &n); err != nil {
		t.Fatalf("load node: %v", err)
	}
	return buildPositionIndex(&n)
}

func TestPositionIndexMappingAndSequence(t *testing.T) {
	pi := testIndex(t, "node:\n  id: n\npoints:\n  - key: opening\n    access: r\n")
	line, col := pi.at(fieldPath(fieldPath(nil, "node"), "id"))
	if line != 2 || col != 3 {
		t.Fatalf("node.id = %d:%d, want 2:3", line, col)
	}
	line, col = pi.at(indexPath(fieldPath(nil, "points"), 0))
	if line != 4 || col < 1 {
		t.Fatalf("points[0] = %d:%d, want line 4", line, col)
	}
	line, col = pi.at(fieldPath(indexPath(fieldPath(nil, "points"), 0), "access"))
	if line != 5 || col != 5 {
		t.Fatalf("points[0].access = %d:%d, want 5:5", line, col)
	}
}

func TestPositionIndexFlowMapping(t *testing.T) {
	pi := testIndex(t, "node: {id: n, product_key: p}\n")
	line, col := pi.at(fieldPath(fieldPath(nil, "node"), "id"))
	if line != 1 || col <= 1 {
		t.Fatalf("flow node.id = %d:%d", line, col)
	}
	if key, ok := pi.keyAt(line, col); !ok || key != "id" {
		t.Fatalf("keyAt = %q,%v want id,true", key, ok)
	}
}

func TestPositionIndexMissingFallsBackToAncestor(t *testing.T) {
	pi := testIndex(t, "node:\n  product_key: p\n")
	line, col := pi.at(fieldPath(fieldPath(nil, "node"), "id"))
	if line != 1 || col != 1 {
		t.Fatalf("missing node.id = %d:%d, want node key 1:1", line, col)
	}
	line, col = pi.at(fieldPath(nil, "absent"))
	if line != 1 || col != 1 {
		t.Fatalf("absent root = %d:%d, want 1:1", line, col)
	}
}

func TestPositionIndexAliasAndMerge(t *testing.T) {
	src := "base: &b\n  id: from-anchor\n  name: anchor\nuse:\n  <<: *b\n  id: explicit\n"
	pi := testIndex(t, src)
	// Explicit keys win over merged keys.
	line, col := pi.at(fieldPath(fieldPath(nil, "use"), "id"))
	if line != 6 || col != 3 {
		t.Fatalf("explicit id = %d:%d, want 6:3", line, col)
	}
	// Merged keys resolve to the anchor definition.
	line, col = pi.at(fieldPath(fieldPath(nil, "use"), "name"))
	if line != 3 || col != 3 {
		t.Fatalf("merged name = %d:%d, want 3:3", line, col)
	}
}

func TestPositionIndexMergeSequenceFirstWins(t *testing.T) {
	src := "a: &a {x: 1}\nb: &b {x: 2, y: 2}\nm:\n  <<: [*a, *b]\n"
	pi := testIndex(t, src)
	line, col := pi.at(fieldPath(fieldPath(nil, "m"), "x"))
	if line != 1 || col < 1 {
		t.Fatalf("merged x should point at anchor a on line 1, got %d:%d", line, col)
	}
	line, col = pi.at(fieldPath(fieldPath(nil, "m"), "y"))
	if line != 2 || col < 1 {
		t.Fatalf("merged y should point at anchor b on line 2, got %d:%d", line, col)
	}
}

func TestLoadErrorDispatchOrder(t *testing.T) {
	var out struct {
		A int `yaml:"a"`
		B int `yaml:"b"`
	}
	src := "a: x\nb: y\nunknown1: 1\nunknown2: 2\n"
	err := yaml.Load([]byte(src), &out, yaml.WithKnownFields(), yaml.WithUniqueKeys())
	if err == nil {
		t.Fatal("expected errors")
	}
	ds := classifyYAMLError("x.yaml", err, nil)
	if len(ds) < 4 {
		t.Fatalf("diagnostics collapsed: %v", ds)
	}
}

func TestSyntaxErrorUsesProblemMark(t *testing.T) {
	var out struct {
		A []int `yaml:"a"`
	}
	src := "a: [1, 2\n"
	err := yaml.Load([]byte(src), &out)
	if err == nil {
		t.Fatal("expected syntax error")
	}
	ds := classifyYAMLError("x.yaml", err, nil)
	if len(ds) != 1 || ds[0].RuleID != RuleSyntax {
		t.Fatalf("ds = %v", ds)
	}
	if ds[0].Line != 2 || ds[0].Col != 1 {
		t.Fatalf("syntax position = %d:%d, want Mark 2:1", ds[0].Line, ds[0].Col)
	}
	if !strings.Contains(ds[0].Message, "flow sequence") {
		t.Fatalf("context missing: %q", ds[0].Message)
	}
}

func TestEmptyAndMultiDocument(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		rule string
		msg  string
	}{
		{"empty", "", RuleEmptyDocument, "empty document"},
		{"multi", "a: 1\n---\na: 2\n", RuleSyntax, "multiple"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out struct {
				A int `yaml:"a"`
			}
			err := yaml.Load([]byte(tc.src), &out)
			ds := classifyYAMLError("x.yaml", err, nil)
			if len(ds) != 1 || ds[0].RuleID != tc.rule || !strings.Contains(ds[0].Message, tc.msg) {
				t.Fatalf("ds = %v", ds)
			}
			if ds[0].Line != 1 || ds[0].Col != 1 {
				t.Fatalf("position = %d:%d, want 1:1", ds[0].Line, ds[0].Col)
			}
		})
	}
}

func TestNullDocumentIsEmpty(t *testing.T) {
	var n yaml.Node
	if err := yaml.Load([]byte("null\n"), &n); err != nil {
		t.Fatalf("null should parse: %v", err)
	}
	if !emptyNode(&n) {
		t.Fatal("null document must be treated as empty")
	}
}

func TestTypeErrorRedactsValue(t *testing.T) {
	var out struct {
		A int `yaml:"a"`
	}
	err := yaml.Load([]byte("a: SENTINEL\n"), &out)
	if err == nil {
		t.Fatal("expected type error")
	}
	ds := classifyYAMLError("x.yaml", err, nil)
	if len(ds) != 1 || ds[0].RuleID != RuleTypeMismatch {
		t.Fatalf("ds = %v", ds)
	}
	if strings.Contains(ds[0].Message, "SENTINEL") {
		t.Fatalf("value leaked: %q", ds[0].Message)
	}
}
