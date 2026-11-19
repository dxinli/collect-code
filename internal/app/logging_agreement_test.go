package app

import (
	"fmt"
	"testing"

	"github.com/dxinli/collect-code/internal/logging"
)

// The V62/V63 rules and logging.ParseOptions must accept exactly the same
// values; otherwise config-check could pass a config that run rejects.
func TestLogOptionAgreement(t *testing.T) {
	for _, level := range []string{"", "debug", "info", "INFO", "warn", "error", "trace"} {
		dir := writeConfig(t, map[string]string{
			"config.yaml": "node:\n  id: n\n  product_key: p\n" +
				"northbound:\n  mqtt:\n    broker: mqtt://x\n" +
				fmt.Sprintf("log:\n  level: %q\n", level),
		})
		code, _, stderr := run(t, []string{"config-check", "--config", dir}, nil)
		_, err := logging.ParseOptions(level, "json", nil)
		if (code == ExitOK) != (err == nil) {
			t.Errorf("level %q: config-check code=%d ParseOptions err=%v (stderr=%s)",
				level, code, err, stderr)
		}
	}

	for _, format := range []string{"", "json", "text"} {
		dir := writeConfig(t, map[string]string{
			"config.yaml": "node:\n  id: n\n  product_key: p\n" +
				"northbound:\n  mqtt:\n    broker: mqtt://x\n" +
				fmt.Sprintf("log:\n  format: %q\n", format),
		})
		code, _, stderr := run(t, []string{"config-check", "--config", dir}, nil)
		_, err := logging.ParseOptions("info", format, nil)
		if (code == ExitOK) != (err == nil) {
			t.Errorf("format %q: config-check code=%d ParseOptions err=%v (stderr=%s)",
				format, code, err, stderr)
		}
	}
}
