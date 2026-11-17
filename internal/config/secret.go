package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"go.yaml.in/yaml/v4"
)

type secretSource uint8

const (
	secretUnset secretSource = iota
	secretLiteral
	secretEnv
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Secret holds a credential that must never appear in logs, summaries or
// generic marshaling. A YAML scalar of the form "env:NAME" is resolved from
// the environment at load time; any other scalar is a literal value.
type Secret struct {
	value   string
	source  secretSource
	envName string
}

// UnmarshalYAML is the v4 hook. v4 does not call it for a null node, which
// means "password: null" leaves the zero value (unset) exactly as intended.
func (s *Secret) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("secret must be a scalar")
	}
	if node.Tag == "!!null" {
		*s = Secret{}
		return nil
	}
	raw := node.Value
	if len(raw) >= 4 && raw[:4] == "env:" {
		s.source = secretEnv
		s.envName = raw[4:]
		return nil
	}
	s.source = secretLiteral
	s.value = raw
	return nil
}

// String implements fmt.Stringer so %v never reveals the value.
func (s Secret) String() string { return "[REDACTED]" }

// GoString implements fmt.GoStringer so %#v never reveals the value.
func (s Secret) GoString() string { return "[REDACTED]" }

// MarshalJSON prevents accidental serialization of the value.
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal("[REDACTED]") }

// Reveal returns the plaintext. Callers must be credential consumers only;
// 01a has no caller.
func (s Secret) Reveal() string { return s.value }

// IsSet reports whether a non-empty value is present.
func (s Secret) IsSet() bool { return s.value != "" }

// Masked renders the value state for summaries.
func (s Secret) Masked() string {
	switch s.source {
	case secretEnv:
		if s.value == "" {
			return "[unset]"
		}
		return "[set via env " + s.envName + "]"
	case secretLiteral:
		if s.value == "" {
			return "[unset]"
		}
		return "[set]"
	default:
		return "[unset]"
	}
}

func (s *Secret) resolve(lookup func(string) (string, bool)) error {
	if s.source != secretEnv {
		return nil
	}
	if s.envName == "" {
		return fmt.Errorf("env: requires a variable name")
	}
	if !envNamePattern.MatchString(s.envName) {
		// Deliberately do not echo the name: a user may have pasted a
		// literal token into the variable-name position.
		return fmt.Errorf("invalid environment variable name")
	}
	v, ok := lookup(s.envName)
	if !ok {
		return fmt.Errorf("environment variable %s is not set", s.envName)
	}
	if v == "" {
		*s = Secret{}
		return nil
	}
	s.value = v
	return nil
}

// resolveToken resolves chirpstack.api_token_env into the private token by
// reusing the same Secret resolution rules as every other credential.
func (c *ChirpstackConfig) resolveToken(lookup func(string) (string, bool)) error {
	if c.APITokenEnv == "" {
		c.apiToken = Secret{}
		return nil
	}
	s := Secret{source: secretEnv, envName: c.APITokenEnv}
	if err := s.resolve(lookup); err != nil {
		return err
	}
	c.apiToken = s
	return nil
}

// TokenMasked reports the token state for summaries.
func (c ChirpstackConfig) TokenMasked() string { return c.apiToken.Masked() }

var defaultLookupEnv = os.LookupEnv
