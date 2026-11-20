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
	"sync"
	"syscall"
	"time"

	"github.com/dxinli/collect-code/internal/config"
	"github.com/dxinli/collect-code/internal/core/engine"
	"github.com/dxinli/collect-code/internal/interfaces"
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

// SimulateFunc runs the simulator subcommand. It is injected by cmd so the
// app package never imports a concrete simulator.
type SimulateFunc func(ctx context.Context, args []string, stdout, stderr io.Writer) int

// Deps carries the runtime dependencies the command line assembles. Only the
// "run" command requires a Clock; "simulate" requires Simulate; version and
// config-check ignore both.
type Deps struct {
	Clock    interfaces.Clock
	Simulate SimulateFunc
}

// Run executes one command and returns the process exit code.
func Run(ctx context.Context, deps Deps, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
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
		return runService(ctx, deps, args[1:], stderr, lookupEnv)
	case "simulate":
		return runSimulate(ctx, deps, args[1:], stdout, stderr)
	case "-h", "--help":
		usage(stdout)
		return ExitOK
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return ExitUsage
	}
}

func runVersion(w io.Writer) int {
	_, _ = fmt.Fprintf(w, "binary: collect-code\n")
	_, _ = fmt.Fprintf(w, "version: %s\n", version.Version)
	_, _ = fmt.Fprintf(w, "commit: %s\n", version.Commit)
	_, _ = fmt.Fprintf(w, "build_date: %s\n", version.BuildDate)
	_, _ = fmt.Fprintf(w, "go: %s\n", runtime.Version())
	_, _ = fmt.Fprintf(w, "platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return ExitOK
}

func runConfigCheck(args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	dir, code, ok := parseConfigFlag("config-check", args, stderr)
	if !ok {
		return code
	}
	bundle, _, err := loadAndValidate(dir, lookupEnv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return ExitError
	}
	_, _ = fmt.Fprintln(stdout, bundle.Summary())
	return ExitOK
}

// loadAndValidate is the single place that combines loading with the shared
// logging option validation, so config-check and run cannot disagree.
func loadAndValidate(dir string, lookupEnv func(string) (string, bool)) (*config.Bundle, logging.Options, error) {
	bundle, err := config.Load(dir, config.WithEnv(lookupEnv))
	if err != nil {
		return nil, logging.Options{}, err
	}
	opts, err := logging.ParseOptions(
		bundle.Config.Log.Level,
		bundle.Config.Log.Format,
		bundle.Config.Log.RedactFields,
	)
	if err != nil {
		return nil, logging.Options{}, err
	}
	return bundle, opts, nil
}

func runSimulate(ctx context.Context, deps Deps, args []string, stdout, stderr io.Writer) int {
	if deps.Simulate == nil {
		_, _ = fmt.Fprintln(stderr, "simulate is unavailable: no simulator dependency was provided")
		return ExitError
	}
	return deps.Simulate(ctx, args, stdout, stderr)
}

func runService(ctx context.Context, deps Deps, args []string, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	dir, code, ok := parseConfigFlag("run", args, stderr)
	if !ok {
		return code
	}
	if deps.Clock == nil {
		_, _ = fmt.Fprintln(stderr, "clock is required")
		return ExitError
	}
	bundle, opts, err := loadAndValidate(dir, lookupEnv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return ExitError
	}
	opts.Output = stderr
	opts.RedactValues = bundle.SecretValues()
	logger := logging.New(opts)

	eng, err := engine.New(deps.Clock, logger)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return ExitError
	}

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
		"started_at", eng.StartedAt().UTC().Format(time.RFC3339Nano),
	)
	logger.Warn("skeleton mode: no drivers or channels wired", "reason", "skeleton")

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var shutdownOnce sync.Once
	logShutdown := func(reason string, extra ...any) {
		shutdownOnce.Do(func() {
			attrs := []any{"event", "shutdown", "component", "app", "reason", reason}
			attrs = append(attrs, extra...)
			logger.Info("shutdown", attrs...)
		})
	}

	go func() {
		select {
		case sig := <-sigCh:
			// Restore the default action so a second signal can terminate a
			// shutdown that is taking too long.
			signal.Stop(sigCh)
			logShutdown("signal", "signal", sig.String())
			cancel()
		case <-runCtx.Done():
		}
	}()

	runEngine(runCtx, eng, logger)
	// Reached on parent-context cancellation; a signal path already logged.
	logShutdown("context")
	return ExitOK
}

// runEngine is the future home of the scheduler/collector. In 02a it runs the
// lifecycle component so later components inherit one shutdown path.
func runEngine(ctx context.Context, eng *engine.Engine, logger *slog.Logger) {
	if err := eng.Run(ctx); err != nil {
		logger.Error("engine stopped with error", "component", "engine", "error", err)
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
		_, _ = fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		return "", ExitUsage, false
	}
	if *dir == "" {
		_, _ = fmt.Fprintln(stderr, "--config must not be empty")
		return "", ExitUsage, false
	}
	return *dir, ExitOK, true
}

func usage(w io.Writer) {
	_, _ = fmt.Fprint(w, `collect-code — intelligent edge collector

Usage:
  collect-code <command> [flags]

Commands:
  run            start the collector (skeleton mode in 01a)
  simulate       run the in-process MQTT broker and cloud stub
  config-check   parse and validate the configuration, print a summary
  version        print build information

Flags:
  --config <dir> configuration directory (default `+defaultConfigDir+`)

Exit codes:
  0 success  1 configuration/runtime error  2 usage error
`)
}
