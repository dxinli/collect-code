// Package logging provides the JSON logging contract, the reserved context
// fields and credential redaction.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
)

const redacted = "[REDACTED]"

// Options is the resolved logging configuration.
type Options struct {
	Level        slog.Level
	Format       string
	Output       io.Writer
	RedactFields []string
	RedactValues []string
}

// ParseOptions validates the config primitives shared by config-check and
// run, so a config accepted by one cannot be rejected by the other.
func ParseOptions(level, format string, redact []string) (Options, error) {
	lv, err := parseLevel(level)
	if err != nil {
		return Options{}, err
	}
	if format != "json" {
		return Options{}, fmt.Errorf("unsupported log.format %q (only json in v0.1)", format)
	}
	return Options{
		Level:        lv,
		Format:       format,
		Output:       os.Stderr,
		RedactFields: append([]string(nil), redact...),
	}, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log.level %q (want debug|info|warn|error)", s)
	}
}

// New builds a JSON logger with key- and value-level redaction. Message
// sanitisation is applied by a thin wrapper because ReplaceAttr only sees
// attributes, not the message.
func New(o Options) *slog.Logger {
	out := o.Output
	if out == nil {
		out = os.Stderr
	}
	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level:       o.Level,
		ReplaceAttr: redactor(o.RedactFields, o.RedactValues),
	})
	if len(o.RedactValues) == 0 {
		return slog.New(handler)
	}
	return slog.New(&messageSanitizer{Handler: handler, values: o.RedactValues})
}

func redactor(fields, values []string) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, a slog.Attr) slog.Attr {
		if matchesField(a.Key, fields) {
			return slog.String(a.Key, redacted)
		}

		v := a.Value.Resolve()
		switch v.Kind() {
		case slog.KindString:
			s := v.String()
			if containsSecretExact(s, values) {
				return slog.String(a.Key, redacted)
			}
			if strings.Contains(s, "://") {
				u, err := url.Parse(s)
				if err != nil {
					// An unparseable URL may embed credentials; never print it.
					return slog.String(a.Key, redacted)
				}
				if u.User != nil {
					u.User = url.User(redacted)
					return slog.String(a.Key, u.String())
				}
			}
		case slog.KindAny:
			if s, ok := anyString(v.Any()); ok && containsSecret(s, values) {
				return slog.String(a.Key, redacted)
			}
		}
		return a
	}
}

func matchesField(key string, fields []string) bool {
	k := strings.ToLower(key)
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" {
			continue
		}
		if k == f || strings.HasSuffix(k, "_"+f) || strings.HasSuffix(k, "."+f) {
			return true
		}
	}
	return false
}

func containsSecretExact(s string, secrets []string) bool {
	for _, secret := range secrets {
		if secret != "" && s == secret {
			return true
		}
	}
	return false
}

func containsSecret(s string, secrets []string) bool {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(s, secret) {
			return true
		}
	}
	return false
}

func anyString(v any) (string, bool) {
	switch x := v.(type) {
	case error:
		return x.Error(), true
	case fmt.Stringer:
		return x.String(), true
	default:
		return "", false
	}
}

// messageSanitizer replaces registered credential values in log messages.
type messageSanitizer struct {
	slog.Handler
	values []string
}

func (h *messageSanitizer) Handle(ctx context.Context, r slog.Record) error {
	msg := r.Message
	for _, v := range h.values {
		if v != "" {
			msg = strings.ReplaceAll(msg, v, redacted)
		}
	}
	if msg != r.Message {
		r = r.Clone()
		r.Message = msg
	}
	return h.Handler.Handle(ctx, r)
}

func (h *messageSanitizer) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &messageSanitizer{Handler: h.Handler.WithAttrs(attrs), values: h.values}
}

func (h *messageSanitizer) WithGroup(name string) slog.Handler {
	return &messageSanitizer{Handler: h.Handler.WithGroup(name), values: h.values}
}
