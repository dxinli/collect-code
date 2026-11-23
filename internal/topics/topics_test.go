package topics_test

// Covers: EXT-TOPIC-01, EXT-LWT-01 (topic validation), N5 (reserved prefixes).

import (
	"errors"
	"strings"
	"testing"

	"github.com/dxinli/collect-code/internal/topics"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		vars map[string]string
		want string
	}{
		{
			name: "default client id template",
			tmpl: "collector-{node.id}",
			vars: map[string]string{"node.id": "gateway-001"},
			want: "collector-gateway-001",
		},
		{
			name: "node_id alias",
			tmpl: "collector-{node_id}",
			vars: map[string]string{"node_id": "gateway-001"},
			want: "collector-gateway-001",
		},
		{
			name: "multiple placeholders",
			tmpl: "{product_key}-{node.id}",
			vars: map[string]string{"product_key": "farm-a", "node.id": "gw-1"},
			want: "farm-a-gw-1",
		},
		{
			name: "literal braces are not reparsed",
			tmpl: "{a}",
			vars: map[string]string{"a": "x{y}"},
			want: "x{y}",
		},
		{
			name: "empty template",
			tmpl: "",
			vars: nil,
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := topics.Render(tc.tmpl, tc.vars)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Render = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderErrors(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		vars map[string]string
	}{
		{name: "unknown placeholder", tmpl: "{missing}", vars: map[string]string{}},
		{name: "empty placeholder", tmpl: "a{}b", vars: map[string]string{}},
		{name: "unmatched open", tmpl: "a{b", vars: map[string]string{"b": "x"}},
		{name: "unmatched close", tmpl: "a}b", vars: nil},
		{name: "invalid placeholder char", tmpl: "{a-b}", vars: map[string]string{"a-b": "x"}},
		{name: "empty value", tmpl: "{a}", vars: map[string]string{"a": ""}},
		{name: "NUL value", tmpl: "{a}", vars: map[string]string{"a": "x\x00y"}},
		{name: "invalid UTF-8 value", tmpl: "{a}", vars: map[string]string{"a": string([]byte{0xff})}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := topics.Render(tc.tmpl, tc.vars); err == nil {
				t.Fatalf("Render(%q) = nil error, want error", tc.tmpl)
			}
		})
	}
}

func TestRenderTopic(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		vars map[string]string
		want string
	}{
		{
			name: "default node state template",
			tmpl: "v1/{product_key}/$nodes/{node_id}/state",
			vars: map[string]string{"product_key": "irrigation", "node_id": "gateway-001"},
			want: "v1/irrigation/$nodes/gateway-001/state",
		},
		{
			name: "non-default prefix and product key",
			tmpl: "custom/v2/{product_key}/nodes/{node_id}/offline",
			vars: map[string]string{"product_key": "farm-a", "node_id": "gateway-001"},
			want: "custom/v2/farm-a/nodes/gateway-001/offline",
		},
		{
			name: "node.id alias",
			tmpl: "v1/{product_key}/$nodes/{node.id}/state",
			vars: map[string]string{"product_key": "p", "node.id": "n"},
			want: "v1/p/$nodes/n/state",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := topics.RenderTopic(tc.tmpl, tc.vars)
			if err != nil {
				t.Fatalf("RenderTopic: %v", err)
			}
			if got != tc.want {
				t.Fatalf("RenderTopic = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderTopicRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "slash changes hierarchy", value: "a/b"},
		{name: "plus wildcard", value: "a+"},
		{name: "hash wildcard", value: "a#"},
		{name: "NUL", value: "a\x00b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := topics.RenderTopic("v1/{product_key}/x", map[string]string{"product_key": tc.value})
			if err == nil {
				t.Fatalf("RenderTopic(value %q) = nil error, want error", tc.value)
			}
		})
	}
}

func TestRenderTopicRejectsInvalidResult(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		vars map[string]string
	}{
		{name: "empty level", tmpl: "v1//x/{product_key}", vars: map[string]string{"product_key": "p"}},
		{name: "leading slash", tmpl: "/v1/{product_key}", vars: map[string]string{"product_key": "p"}},
		{name: "trailing slash", tmpl: "v1/{product_key}/", vars: map[string]string{"product_key": "p"}},
		{name: "reserved sys prefix", tmpl: "$SYS/{product_key}", vars: map[string]string{"product_key": "p"}},
		{name: "reserved share prefix", tmpl: "$share/{product_key}", vars: map[string]string{"product_key": "p"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := topics.RenderTopic(tc.tmpl, tc.vars); err == nil {
				t.Fatalf("RenderTopic(%q) = nil error, want error", tc.tmpl)
			}
		})
	}
}

func TestRenderFilter(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		vars map[string]string
		want string
	}{
		{
			name: "plus wildcard in template",
			tmpl: "v1/{product_key}/+/service/invoke",
			vars: map[string]string{"product_key": "irrigation"},
			want: "v1/irrigation/+/service/invoke",
		},
		{
			name: "hash wildcard in template",
			tmpl: "v1/{product_key}/#",
			vars: map[string]string{"product_key": "irrigation"},
			want: "v1/irrigation/#",
		},
		{
			name: "multi-level filter",
			tmpl: "v1/{product_key}/{device_sn}/service/invoke",
			vars: map[string]string{"product_key": "p", "device_sn": "VALVE-0001"},
			want: "v1/p/VALVE-0001/service/invoke",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := topics.RenderFilter(tc.tmpl, tc.vars)
			if err != nil {
				t.Fatalf("RenderFilter: %v", err)
			}
			if got != tc.want {
				t.Fatalf("RenderFilter = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderFilterRejectsValueWildcards(t *testing.T) {
	for _, value := range []string{"a+", "a#", "a/b", "a\x00b"} {
		t.Run(value, func(t *testing.T) {
			_, err := topics.RenderFilter("v1/{device_sn}/service/invoke", map[string]string{"device_sn": value})
			if err == nil {
				t.Fatalf("RenderFilter(value %q) = nil error, want error", value)
			}
		})
	}
}

func TestValidatePublishTopic(t *testing.T) {
	valid := []string{
		"v1/irrigation/VALVE-0001/property/post",
		"v1/irrigation/$nodes/gateway-001/state",
		"a",
		"a/b/c",
	}
	for _, topic := range valid {
		t.Run("valid/"+topic, func(t *testing.T) {
			if err := topics.ValidatePublishTopic(topic); err != nil {
				t.Fatalf("ValidatePublishTopic(%q): %v", topic, err)
			}
		})
	}

	invalid := map[string]string{
		"empty":              "",
		"plus wildcard":      "v1/+/x",
		"hash wildcard":      "v1/#",
		"empty middle":       "v1//x",
		"leading slash":      "/v1/x",
		"trailing slash":     "v1/x/",
		"NUL":                "v1/\x00/x",
		"invalid UTF-8":      "v1/" + string([]byte{0xff}),
		"too long":           strings.Repeat("a", 65536),
		"reserved $SYS":      "$SYS/broker",
		"reserved $SYS bare": "$SYS",
		"reserved $share":    "$share/g/topic",
		"reserved $queue":    "$queue/topic",
	}
	for name, topic := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			if err := topics.ValidatePublishTopic(topic); err == nil {
				t.Fatalf("ValidatePublishTopic(%q) = nil error, want error", topic)
			}
		})
	}
}

func TestValidateFilter(t *testing.T) {
	valid := []string{
		"v1/irrigation/+/service/invoke",
		"v1/irrigation/#",
		"v1/irrigation/VALVE-0001/service/invoke",
		"#",
		"+",
		"a/+/b/#",
	}
	for _, filter := range valid {
		t.Run("valid/"+filter, func(t *testing.T) {
			if err := topics.ValidateFilter(filter); err != nil {
				t.Fatalf("ValidateFilter(%q): %v", filter, err)
			}
		})
	}

	invalid := map[string]string{
		"empty":            "",
		"hash not last":    "v1/#/x",
		"hash with suffix": "v1/x#",
		"plus with suffix": "v1/x+",
		"empty middle":     "v1//x",
		"leading slash":    "/v1/x",
		"trailing slash":   "v1/x/",
		"NUL":              "v1/\x00/x",
		"invalid UTF-8":    "v1/" + string([]byte{0xff}),
		"too long":         strings.Repeat("a", 65536),
		"reserved $SYS":    "$SYS/#",
		"reserved $share":  "$share/group/topic",
		"reserved $queue":  "$queue/topic",
	}
	for name, filter := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			if err := topics.ValidateFilter(filter); err == nil {
				t.Fatalf("ValidateFilter(%q) = nil error, want error", filter)
			}
		})
	}
}

func TestSentinelErrors(t *testing.T) {
	_, err := topics.RenderTopic("v1/+/x", map[string]string{})
	if !errors.Is(err, topics.ErrInvalidTemplate) && !errors.Is(err, topics.ErrInvalidTopic) {
		t.Fatalf("RenderTopic error %v does not wrap a topics sentinel", err)
	}
	err = topics.ValidateFilter("v1//x")
	if !errors.Is(err, topics.ErrInvalidTopic) {
		t.Fatalf("ValidateFilter error %v does not wrap ErrInvalidTopic", err)
	}
}
