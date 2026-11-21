package sim

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2

	simulateShutdownTimeout = 5 * time.Second
)

type simulateOptions struct {
	manifest        string
	httpAddr        string
	mqttAddr        string
	captureCapacity int
	topicFilter     string
}

// Run implements the simulate subcommand. It is injected into app.Deps by
// cmd/collect-code so the app package never imports the simulator.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	opts, code, ok := parseSimulateFlags(args, stdout, stderr)
	if !ok {
		return code
	}
	if err := ctx.Err(); err != nil {
		return exitOK
	}

	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mqttLogger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	warnNonLoopback(logger, "http", opts.httpAddr)
	warnNonLoopback(logger, "mqtt", opts.mqttAddr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var shutdownOnce sync.Once
	logShutdown := func(reason string) {
		shutdownOnce.Do(func() {
			logger.Info("shutdown", "event", "shutdown", "component", "simulate", "reason", reason)
		})
	}
	go func() {
		select {
		case <-sigCh:
			// Restore the default action so a second signal can terminate a
			// shutdown that is taking too long.
			signal.Stop(sigCh)
			logShutdown("signal")
			cancel()
		case <-runCtx.Done():
		}
	}()

	runtime, err := NewRuntime(RuntimeOptions{
		HTTPAddr:        opts.httpAddr,
		CaptureCapacity: opts.captureCapacity,
		Logger:          logger,
	})
	if err != nil {
		logger.Error("simulate failed", "event", "error", "error", err)
		return exitError
	}
	broker, err := NewMQTTBroker(MQTTBrokerOptions{
		Addr:        opts.mqttAddr,
		TopicFilter: opts.topicFilter,
		Runtime:     runtime,
		Logger:      mqttLogger,
	})
	if err != nil {
		logger.Error("simulate failed", "event", "error", "error", err)
		_ = runtime.Close(context.Background())
		return exitError
	}
	if err := broker.Register(runtime); err != nil {
		logger.Error("simulate failed", "event", "error", "error", err)
		_ = broker.Close()
		_ = runtime.Close(context.Background())
		return exitError
	}
	if err := broker.Start(); err != nil {
		logger.Error("simulate failed", "event", "error", "error", err)
		_ = broker.Close()
		_ = runtime.Close(context.Background())
		return exitError
	}
	if err := runtime.Start(runCtx); err != nil {
		logger.Error("simulate failed", "event", "error", "error", err)
		_ = broker.Close()
		_ = runtime.Close(context.Background())
		return exitError
	}
	if err := runtime.WriteManifest(opts.manifest); err != nil {
		logger.Error("simulate failed", "event", "error", "error", err)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), simulateShutdownTimeout)
		_ = runtime.Close(shutdownCtx)
		shutdownCancel()
		_ = broker.Close()
		return exitError
	}

	logger.Info("startup",
		"event", "startup",
		"component", "simulate",
		"http", runtime.BaseURL(),
		"mqtt", broker.URL(),
		"manifest", opts.manifest,
	)

	<-runCtx.Done()
	logShutdown("context")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), simulateShutdownTimeout)
	defer shutdownCancel()
	if err := runtime.Close(shutdownCtx); err != nil {
		logger.Error("simulate shutdown", "component", "http", "error", err)
	}
	if err := broker.Close(); err != nil {
		logger.Error("simulate shutdown", "component", "mqtt", "error", err)
	}
	return exitOK
}

func parseSimulateFlags(args []string, stdout, stderr io.Writer) (simulateOptions, int, bool) {
	fs := flag.NewFlagSet("simulate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	opts := simulateOptions{}
	fs.StringVar(&opts.manifest, "manifest", "", "write a startup manifest to this path")
	fs.StringVar(&opts.httpAddr, "http-addr", defaultHTTPAddr, "simulator HTTP address")
	fs.StringVar(&opts.mqttAddr, "mqtt-addr", defaultMQTTAddr, "MQTT broker address")
	fs.IntVar(&opts.captureCapacity, "capture-capacity", defaultCaptureCapacity, "uplink capture ring size")
	fs.StringVar(&opts.topicFilter, "topic-filter", defaultTopicFilter, "cloud-stub subscription filter")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeSimulateUsage(stdout)
			return simulateOptions{}, exitOK, false
		}
		_, _ = fmt.Fprintf(stderr, "simulate: %v\n", err)
		writeSimulateUsage(stderr)
		return simulateOptions{}, exitUsage, false
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "simulate: unexpected arguments: %v\n", fs.Args())
		writeSimulateUsage(stderr)
		return simulateOptions{}, exitUsage, false
	}
	if opts.captureCapacity < 1 || opts.captureCapacity > maxCaptureCapacity {
		_, _ = fmt.Fprintf(stderr, "simulate: --capture-capacity must be between 1 and %d\n", maxCaptureCapacity)
		return simulateOptions{}, exitUsage, false
	}
	if err := validateListenAddr(opts.httpAddr); err != nil {
		_, _ = fmt.Fprintf(stderr, "simulate: --http-addr: %v\n", err)
		return simulateOptions{}, exitUsage, false
	}
	if err := validateListenAddr(opts.mqttAddr); err != nil {
		_, _ = fmt.Fprintf(stderr, "simulate: --mqtt-addr: %v\n", err)
		return simulateOptions{}, exitUsage, false
	}
	if !mqtt.IsValidFilter(opts.topicFilter, false) {
		_, _ = fmt.Fprintf(stderr, "simulate: --topic-filter %q is not a valid MQTT filter\n", opts.topicFilter)
		return simulateOptions{}, exitUsage, false
	}
	return opts, exitOK, true
}

func validateListenAddr(addr string) error {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("address %q must include a host", addr)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return fmt.Errorf("invalid port in %q", addr)
	}
	return nil
}

func warnNonLoopback(logger *slog.Logger, name, addr string) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	if host == "localhost" {
		return
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return
	}
	logger.Warn("simulator binds a non-loopback address without authentication",
		"component", "simulate", "endpoint", name, "addr", addr)
}

func writeSimulateUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `collect-code simulate — in-process MQTT broker and cloud stub

Usage:
  collect-code simulate [flags]

Flags:
  --manifest <path>        write a startup manifest with actual addresses
  --http-addr <host:port>  simulator HTTP address (default `+defaultHTTPAddr+`)
  --mqtt-addr <host:port>  MQTT broker address (default `+defaultMQTTAddr+`)
  --capture-capacity <n>   uplink capture ring size (default `+strconv.Itoa(defaultCaptureCapacity)+`, 1..1000000)
  --topic-filter <filter>  cloud-stub subscription filter (default "#")
  -h, --help               print this help and exit

Endpoints:
  GET  /_sim/captured
  POST /_sim/invoke
  POST /_sim/control

Exit codes:
  0 success  1 startup/runtime error  2 usage error
`)
}
