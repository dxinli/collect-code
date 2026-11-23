// Package topics renders configured MQTT templates and validates publish
// topics and subscription filters. Templates and values come from
// configuration; this package contains no product or node defaults.
package topics

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// maxTopicBytes is the MQTT UTF-8 encoded string limit for topic names
	// and subscription filters.
	maxTopicBytes = 65535
	// maxPlaceholderName bounds the work done per placeholder; MQTT names are
	// short, and an unbounded scan would hide malformed input.
	maxPlaceholderName = 128
)

var (
	// ErrInvalidTemplate reports a malformed template (unmatched or empty
	// placeholder, or an unknown placeholder name).
	ErrInvalidTemplate = errors.New("topics: invalid template")
	// ErrInvalidTopic reports a topic or filter that violates MQTT rules.
	ErrInvalidTopic = errors.New("topics: invalid topic")
)

// Render substitutes {name} placeholders with the values in vars and returns
// the result. It is used for values that are not topic levels (for example a
// client ID). Values must be valid UTF-8, non-empty, and free of NUL.
func Render(tmpl string, vars map[string]string) (string, error) {
	return render(tmpl, vars, valueRules{
		forbid:      "",
		forbidSlash: false,
	})
}

// RenderTopic substitutes {name} placeholders and validates the result as a
// publish topic. Replacement values may not contain '/', '+', '#' or NUL, so
// a device serial cannot change the topic hierarchy or inject a wildcard.
func RenderTopic(tmpl string, vars map[string]string) (string, error) {
	rendered, err := render(tmpl, vars, valueRules{
		forbid:      "+#",
		forbidSlash: true,
	})
	if err != nil {
		return "", err
	}
	if err := ValidatePublishTopic(rendered); err != nil {
		return "", err
	}
	return rendered, nil
}

// RenderFilter substitutes {name} placeholders and validates the result as a
// subscription filter. Replacement values may not contain '/', '+', '#' or
// NUL, while the template itself may contain wildcards, for example
// "v1/{product_key}/+/service/invoke".
func RenderFilter(tmpl string, vars map[string]string) (string, error) {
	rendered, err := render(tmpl, vars, valueRules{
		forbid:      "+#",
		forbidSlash: true,
	})
	if err != nil {
		return "", err
	}
	if err := ValidateFilter(rendered); err != nil {
		return "", err
	}
	return rendered, nil
}

// ValidatePublishTopic checks a topic that the channel will publish to.
func ValidatePublishTopic(topic string) error {
	if err := validateCommon(topic); err != nil {
		return err
	}
	if strings.ContainsAny(topic, "+#") {
		return fmt.Errorf("%w: publish topic %q must not contain wildcards", ErrInvalidTopic, topic)
	}
	if hasReservedPrefix(topic) {
		return fmt.Errorf("%w: publish topic %q uses a reserved prefix", ErrInvalidTopic, topic)
	}
	if err := validateLevels(topic); err != nil {
		return err
	}
	return nil
}

// ValidateFilter checks a subscription filter.
func ValidateFilter(filter string) error {
	if err := validateCommon(filter); err != nil {
		return err
	}
	levels := strings.Split(filter, "/")
	for i, level := range levels {
		if level == "" {
			return fmt.Errorf("%w: filter %q contains an empty level", ErrInvalidTopic, filter)
		}
		if strings.Contains(level, "#") {
			if level != "#" || i != len(levels)-1 {
				return fmt.Errorf("%w: filter %q has a misplaced '#'", ErrInvalidTopic, filter)
			}
		}
		if strings.Contains(level, "+") && level != "+" {
			return fmt.Errorf("%w: filter %q has a misplaced '+'", ErrInvalidTopic, filter)
		}
	}
	switch levels[0] {
	case "$SYS", "$share", "$queue":
		return fmt.Errorf("%w: filter %q uses a reserved prefix", ErrInvalidTopic, filter)
	}
	return nil
}

type valueRules struct {
	// forbid lists characters that must not appear in a replacement value.
	forbid string
	// forbidSlash rejects '/' in a replacement value.
	forbidSlash bool
}

func render(tmpl string, vars map[string]string, rules valueRules) (string, error) {
	var out strings.Builder
	out.Grow(len(tmpl))
	for i := 0; i < len(tmpl); {
		switch tmpl[i] {
		case '}':
			return "", fmt.Errorf("%w: %q has an unmatched '}' at %d", ErrInvalidTemplate, tmpl, i)
		case '{':
			end := strings.IndexByte(tmpl[i+1:], '}')
			if end < 0 {
				return "", fmt.Errorf("%w: %q has an unmatched '{' at %d", ErrInvalidTemplate, tmpl, i)
			}
			end += i + 1
			name := tmpl[i+1 : end]
			if err := validatePlaceholderName(name); err != nil {
				return "", fmt.Errorf("%w: %q placeholder %q: %v", ErrInvalidTemplate, tmpl, name, err)
			}
			value, ok := vars[name]
			if !ok {
				return "", fmt.Errorf("%w: %q has unknown placeholder %q", ErrInvalidTemplate, tmpl, name)
			}
			if err := validateValue(name, value, rules); err != nil {
				return "", err
			}
			out.WriteString(value)
			i = end + 1
		default:
			out.WriteByte(tmpl[i])
			i++
		}
	}
	return out.String(), nil
}

func validatePlaceholderName(name string) error {
	if name == "" {
		return errors.New("empty name")
	}
	if len(name) > maxPlaceholderName {
		return errors.New("name too long")
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.' {
			continue
		}
		return fmt.Errorf("invalid character %q", c)
	}
	return nil
}

func validateValue(name, value string, rules valueRules) error {
	if value == "" {
		return fmt.Errorf("%w: placeholder %q has an empty value", ErrInvalidTemplate, name)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: placeholder %q is not valid UTF-8", ErrInvalidTemplate, name)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%w: placeholder %q contains NUL", ErrInvalidTemplate, name)
	}
	if rules.forbidSlash && strings.IndexByte(value, '/') >= 0 {
		return fmt.Errorf("%w: placeholder %q contains '/'", ErrInvalidTemplate, name)
	}
	if rules.forbid != "" && strings.ContainsAny(value, rules.forbid) {
		return fmt.Errorf("%w: placeholder %q contains a wildcard", ErrInvalidTemplate, name)
	}
	return nil
}

func validateCommon(topic string) error {
	if topic == "" {
		return fmt.Errorf("%w: empty", ErrInvalidTopic)
	}
	if !utf8.ValidString(topic) {
		return fmt.Errorf("%w: %q is not valid UTF-8", ErrInvalidTopic, topic)
	}
	if len(topic) > maxTopicBytes {
		return fmt.Errorf("%w: %q exceeds %d bytes", ErrInvalidTopic, topic, maxTopicBytes)
	}
	if strings.IndexByte(topic, 0) >= 0 {
		return fmt.Errorf("%w: %q contains NUL", ErrInvalidTopic, topic)
	}
	return nil
}

func validateLevels(topic string) error {
	for _, level := range strings.Split(topic, "/") {
		if level == "" {
			return fmt.Errorf("%w: %q contains an empty level", ErrInvalidTopic, topic)
		}
	}
	return nil
}

func hasReservedPrefix(topic string) bool {
	for _, prefix := range []string{"$SYS", "$share", "$queue"} {
		if topic == prefix || strings.HasPrefix(topic, prefix+"/") {
			return true
		}
	}
	return false
}
