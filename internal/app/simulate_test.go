package app

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestSimulateDispatch(t *testing.T) {
	var gotArgs []string
	deps := Deps{Simulate: func(_ context.Context, args []string, stdout, stderr io.Writer) int {
		gotArgs = append([]string(nil), args...)
		_, _ = io.WriteString(stdout, "sim-out")
		_, _ = io.WriteString(stderr, "sim-err")
		return ExitUsage
	}}

	code, stdout, stderr := runWithDeps(t, deps, []string{"simulate", "--manifest", "/tmp/x"}, nil)
	if code != ExitUsage {
		t.Fatalf("code = %d, want %d", code, ExitUsage)
	}
	if got := strings.Join(gotArgs, " "); got != "--manifest /tmp/x" {
		t.Fatalf("args passed to simulator = %q", got)
	}
	if stdout != "sim-out" || stderr != "sim-err" {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestSimulateMissingDependency(t *testing.T) {
	code, _, stderr := runWithDeps(t, Deps{}, []string{"simulate"}, nil)
	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "simulate") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestUsageIncludesSimulate(t *testing.T) {
	code, stdout, _ := runWithDeps(t, Deps{}, []string{"--help"}, nil)
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(stdout, "simulate") {
		t.Fatalf("usage does not mention simulate:\n%s", stdout)
	}
}
