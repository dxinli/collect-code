// Package app wires the command line, configuration and logging together.
// Concrete drivers, channels and adapters are deliberately absent in 01a.
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/dxinli/collect-code/internal/config"
	"github.com/dxinli/collect-code/internal/logging"
	"github.com/dxinli/collect-code/internal/version"
)

const defaultConfigDir = "/etc/collect-code"

// Exit codes are part of the CLI contract used by later tickets.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// Run executes one command and returns the process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	if len(args) == 0 {
		usage(stderr)
		return ExitUsage
	}

	switch args[0] {
	case "version":
		return runVersion(stdout)
	case "config-check":
		return runConfigCheck(args[1:], stdout, stderr, lookupEnv)
	case "run":
		return runService(ctx, args[1:], stderr, lookupEnv)
	case "help", "-h", "--help":
		usage(stdout)
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return ExitUsage
	}
}

func runVersion(w io.Writer) int {
	fmt.Fprintf(w, "binary: collect-code\n")
	fmt.Fprintf(w, "version: %s\n", version.Version)
	fmt.Fprintf(w, "commit: %s\n", version.Commit)
	fmt.Fprintf(w, "build_date: %s\n", version.BuildDate)
	fmt.Fprintf(w, "go: %s\n", runtime.Version())
	fmt.Fprintf(w, "platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return ExitOK
}

func runConfigCheck(args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	dir, code, ok := parseConfigFlag("config-check", args, stderr)
	if !ok {
		return code
	}
	bundle, err := config.Load(dir, config.WithEnv(lookupEnv))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitError
	}
	if _, err := logging.ParseOptions(
		bundle.Config.Log.Level,
		bundle.Config.Log.Format,
		bundle.Config.Log.RedactFields,
	); err != nil {
		fmt.Fprintln(stderr, err)
		return ExitError
	}
	fmt.Fprintln(stdout, bundle.Summary())
	return ExitOK
}

func runService(ctx context.Context, args []string, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	dir, code, ok := parseConfigFlag("run", args, stderr)
	if !ok {
		return code
	}
	bundle, err := config.Load(dir, config.WithEnv(lookupEnv))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitError
	}
	opts, err := logging.ParseOptions(
		bundle.Config.Log.Level,
		bundle.Config.Log.Format,
		bundle.Config.Log.RedactFields,
	)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitError
	}
	opts.Output = stderr
	opts.RedactValues = bundle.SecretValues()
	logger := logging.New(opts)

	// Install the signal handler before the startup line: a test or operator
	// that reacts to "startup" must never race the default signal action.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	signal.Ignore(syscall.SIGHUP)
	defer signal.Reset(syscall.SIGHUP)

	logger.Info("startup",
		"component", "app",
		"node", bundle.Config.Node.ID,
		"version", version.Version,
		"links", len(bundle.Links),
		"devices", len(bundle.Devices),
	)
	logger.Warn("skeleton mode: no drivers or channels wired", "reason", "skeleton")

	runEngine(ctx, sigCh, logger)
	return ExitOK
}

// runEngine is the future home of the scheduler/collector. In 01a it only
// waits for cancellation or a termination signal, so that later components
// inherit the same context and shutdown path.
func runEngine(ctx context.Context, sigCh <-chan os.Signal, logger *slog.Logger) {
	select {
	case sig := <-sigCh:
		logger.Info("shutdown",
			"event", "shutdown",
			"component", "app",
			"reason", "signal",
			"signal", sig.String(),
		)
	case <-ctx.Done():
		logger.Info("shutdown",
			"event", "shutdown",
			"component", "app",
			"reason", "context",
		)
	}
}

func parseConfigFlag(name string, args []string, stderr io.Writer) (string, int, bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("config", defaultConfigDir, "configuration directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", ExitOK, false
		}
		return "", ExitUsage, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return "", ExitUsage, false
	}
	if *dir == "" {
		fmt.Fprintln(stderr, "--config must not be empty")
		return "", ExitUsage, false
	}
	return *dir, ExitOK, true
}

func usage(w io.Writer) {
	fmt.Fprint(w, `collect-code — intelligent edge collector

Usage:
  collect-code <command> [flags]

Commands:
  run            start the collector (skeleton mode in 01a)
  config-check   parse and validate the configuration, print a summary
  version        print build information

Flags:
  --config <dir> configuration directory (default `+defaultConfigDir+`)

Exit codes:
  0 success  1 configuration/runtime error  2 usage error
`)
}
