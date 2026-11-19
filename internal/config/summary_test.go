package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestSummaryRendersAndMasks(t *testing.T) {
	b, err := Load("testdata/config/base")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	const sentinel = "SENTINEL-summary-secret"
	b.Config.Northbound.MQTT.Password = Secret{value: sentinel, source: secretLiteral}
	b.Config.Northbound.MQTT.Broker = "mqtt://user:" + sentinel + "@broker:1883?token=" + sentinel
	b.Links[2].Chirpstack.apiToken = Secret{value: sentinel, source: secretLiteral}

	out := b.Summary()
	if strings.Contains(out, sentinel) {
		t.Fatalf("summary leaked secret:\n%s", out)
	}
	for _, want := range []string{
		`password="[set]"`,
		`broker="mqtt://[REDACTED]@broker:1883"`,
		`chirpstack_api_token="[set]"`,
		`redact_fields=["password","token","authorization","data"]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	if !strings.HasPrefix(out, "config ok: node=gateway-001 product_key=irrigation links=3 devices=4 profiles=3\n") {
		t.Fatalf("first line changed:\n%s", out)
	}
}

func TestSummaryEmptyRedactFields(t *testing.T) {
	b, err := Load("testdata/config/base")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	b.Config.Log.RedactFields = []string{}
	if !strings.Contains(b.Summary(), "redact_fields=[]") {
		t.Fatalf("empty redact list not rendered:\n%s", b.Summary())
	}
}

func TestSummaryDeterministic(t *testing.T) {
	b, err := Load("testdata/config/base")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	first := b.Summary()
	for i := 0; i < 10; i++ {
		if got := b.Summary(); got != first {
			t.Fatalf("summary changed between renders:\n%s\n---\n%s", first, got)
		}
	}
}

func TestEffectiveSummaryHasNoSecretFields(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if typ == nil || seen[typ] {
			return
		}
		seen[typ] = true
		if typ == reflect.TypeOf(Secret{}) {
			t.Fatalf("EffectiveSummary exposes config.Secret")
		}
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(typ.Elem())
		case reflect.Map:
			walk(typ.Key())
			walk(typ.Elem())
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				walk(typ.Field(i).Type)
			}
		}
	}
	walk(reflect.TypeOf(EffectiveSummary{}))
}

func TestSanitizeBroker(t *testing.T) {
	cases := map[string]string{
		"mqtt://user:pass@host:1883":      "mqtt://[REDACTED]@host:1883",
		"mqtt://host:1883?token=secret":   "mqtt://host:1883",
		"mqtts://host:8883/path?x=1#frag": "mqtts://host:8883",
		"not a url":                       "[unparseable broker]",
		"":                                "[unparseable broker]",
		"mqtt://[bad":                     "[unparseable broker]",
	}
	for in, want := range cases {
		if got := sanitizeBroker(in); got != want {
			t.Errorf("sanitizeBroker(%q) = %q, want %q", in, got, want)
		}
	}
}
