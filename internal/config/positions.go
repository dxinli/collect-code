package config

import (
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

// nodePath addresses a value in the YAML document. Mapping steps carry the
// key; sequence steps carry the index. The zero value is the document root.
type nodePath []pathStep

type pathStep struct {
	key     string
	index   int
	isIndex bool
}

// id encodes a path without ambiguity, even when keys contain '/', ':' or
// '['. The key length is part of the encoding.
func (p nodePath) id() string {
	var b strings.Builder
	for _, s := range p {
		if s.isIndex {
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(s.index))
			b.WriteByte(']')
			continue
		}
		b.WriteByte('/')
		b.WriteString(strconv.Itoa(len(s.key)))
		b.WriteByte(':')
		b.WriteString(s.key)
	}
	return b.String()
}

func fieldPath(parent nodePath, key string) nodePath {
	out := make(nodePath, len(parent), len(parent)+1)
	copy(out, parent)
	return append(out, pathStep{key: key})
}

func indexPath(parent nodePath, i int) nodePath {
	out := make(nodePath, len(parent), len(parent)+1)
	copy(out, parent)
	return append(out, pathStep{index: i, isIndex: true})
}

type nodePos struct {
	keyLine, keyCol int
}

type positionIndex struct {
	byPath map[string]nodePos
	byPos  map[[2]int]string
	root   nodePos
}

func buildPositionIndex(doc *yaml.Node) *positionIndex {
	pi := &positionIndex{
		byPath: map[string]nodePos{},
		byPos:  map[[2]int]string{},
	}
	if doc == nil {
		return pi
	}
	root := doc
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return pi
		}
		root = doc.Content[0]
	}
	if root == nil {
		return pi
	}
	pi.root = nodePos{keyLine: root.Line, keyCol: root.Column}
	pi.walk(root, nil, map[*yaml.Node]bool{})
	return pi
}

func (pi *positionIndex) walk(n *yaml.Node, path nodePath, seen map[*yaml.Node]bool) {
	if n == nil || seen[n] {
		return
	}
	seen[n] = true
	defer delete(seen, n)

	switch n.Kind {
	case yaml.AliasNode:
		pi.walk(n.Alias, path, seen)
	case yaml.MappingNode:
		pi.walkMapping(n, path, seen)
	case yaml.SequenceNode:
		for i, item := range n.Content {
			p := indexPath(path, i)
			pi.record(p, item.Line, item.Column)
			pi.walk(item, p, seen)
		}
	}
}

func (pi *positionIndex) walkMapping(n *yaml.Node, path nodePath, seen map[*yaml.Node]bool) {
	// Explicit keys win over merged keys, and earlier merge sources win over
	// later ones (YAML merge semantics, verified with yaml v4).
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode || k.Value == "<<" {
			continue
		}
		p := fieldPath(path, k.Value)
		pi.record(p, k.Line, k.Column)
		pi.recordKey(k)
		pi.walk(v, p, seen)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode || k.Value != "<<" {
			continue
		}
		forEachMergeTarget(v, func(target *yaml.Node) {
			pi.merge(target, path, seen)
		})
	}
}

// forEachMergeTarget yields the anchor mappings of `<<: *A` and
// `<<: [*A, *B]`. Other merge values are ignored.
func forEachMergeTarget(v *yaml.Node, fn func(*yaml.Node)) {
	switch v.Kind {
	case yaml.AliasNode:
		fn(v.Alias)
	case yaml.SequenceNode:
		for _, item := range v.Content {
			if item.Kind == yaml.AliasNode {
				fn(item.Alias)
			}
		}
	}
}

// merge indexes the keys of an anchor mapping only where the destination path
// has no position yet. This matches yaml v4: explicit keys override merged
// ones, and earlier sequence entries override later ones.
func (pi *positionIndex) merge(n *yaml.Node, path nodePath, seen map[*yaml.Node]bool) {
	if n == nil || n.Kind != yaml.MappingNode || seen[n] {
		return
	}
	seen[n] = true
	defer delete(seen, n)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode || k.Value == "<<" {
			continue
		}
		p := fieldPath(path, k.Value)
		if _, exists := pi.byPath[p.id()]; exists {
			continue
		}
		pi.record(p, k.Line, k.Column)
		pi.recordKey(k)
		pi.walk(v, p, seen)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode || k.Value != "<<" {
			continue
		}
		forEachMergeTarget(v, func(target *yaml.Node) {
			pi.merge(target, path, seen)
		})
	}
}

func (pi *positionIndex) record(p nodePath, keyLine, keyCol int) {
	if keyLine <= 0 || keyCol <= 0 {
		return
	}
	pi.byPath[p.id()] = nodePos{keyLine: keyLine, keyCol: keyCol}
}

func (pi *positionIndex) recordKey(k *yaml.Node) {
	if k.Line > 0 && k.Column > 0 {
		key := [2]int{k.Line, k.Column}
		if _, exists := pi.byPos[key]; !exists {
			pi.byPos[key] = k.Value
		}
	}
}

// at returns the key position for the exact path, falling back to the nearest
// existing ancestor and finally to the document root. Line and column are
// always >= 1.
func (pi *positionIndex) at(p nodePath) (line, col int) {
	if pi == nil {
		return 1, 1
	}
	for {
		if pos, ok := pi.byPath[p.id()]; ok {
			return nonZero(pos.keyLine, pos.keyCol)
		}
		if len(p) == 0 {
			break
		}
		p = p[:len(p)-1]
	}
	return nonZero(pi.root.keyLine, pi.root.keyCol)
}

// keyAt resolves the key name at a YAML error mark. Unknown-field errors point
// at the key node, which lets V03 report the key without parsing the decoder's
// Go-type message.
func (pi *positionIndex) keyAt(line, col int) (string, bool) {
	if pi == nil {
		return "", false
	}
	k, ok := pi.byPos[[2]int{line, col}]
	return k, ok
}

func nonZero(line, col int) (int, int) {
	if line <= 0 {
		line = 1
	}
	if col <= 0 {
		col = 1
	}
	return line, col
}
