package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSecretFormattingHidesValue(t *testing.T) {
	s := Secret{value: "s3cr3t", source: secretLiteral}

	for _, got := range []string{
		s.String(),
		fmt.Sprintf("%v", s),
		fmt.Sprintf("%+v", s),
		fmt.Sprintf("%#v", s),
	} {
		if strings.Contains(got, "s3cr3t") {
			t.Fatalf("formatting leaked secret: %q", got)
		}
	}

	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "s3cr3t") {
		t.Fatalf("json leaked secret: %s", raw)
	}
	if string(raw) != `"[REDACTED]"` {
		t.Fatalf("json = %s", raw)
	}
}

func TestSecretMasked(t *testing.T) {
	cases := []struct {
		name string
		in   Secret
		want string
	}{
		{"unset", Secret{}, "[unset]"},
		{"literal", Secret{value: "x", source: secretLiteral}, "[set]"},
		{"literal empty", Secret{value: "", source: secretLiteral}, "[unset]"},
		{"env set", Secret{value: "x", source: secretEnv, envName: "TOKEN"}, "[set via env TOKEN]"},
		{"env unset", Secret{source: secretEnv, envName: "TOKEN"}, "[unset]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.in.Masked(); got != c.want {
				t.Fatalf("masked = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSecretResolve(t *testing.T) {
	s := Secret{source: secretEnv, envName: "TOKEN"}
	if err := s.resolve(func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("expected missing env error")
	}

	s = Secret{source: secretEnv, envName: "TOKEN"}
	if err := s.resolve(func(string) (string, bool) { return "v", true }); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if s.value != "v" || !s.IsSet() {
		t.Fatalf("value = %q", s.value)
	}

	s = Secret{source: secretEnv, envName: "TOKEN"}
	if err := s.resolve(func(string) (string, bool) { return "", true }); err != nil {
		t.Fatalf("empty env should be unset: %v", err)
	}
	if s.IsSet() {
		t.Fatal("empty env should be unset")
	}

	s = Secret{source: secretEnv, envName: "not a name"}
	err := s.resolve(func(string) (string, bool) { return "", true })
	if err == nil || strings.Contains(err.Error(), "not a name") {
		t.Fatalf("invalid name error leaked name: %v", err)
	}

	s = Secret{source: secretEnv, envName: ""}
	if err := s.resolve(func(string) (string, bool) { return "", true }); err == nil {
		t.Fatal("empty env name should fail")
	}
}
