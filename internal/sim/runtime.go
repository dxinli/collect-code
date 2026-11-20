package sim

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultHTTPAddr        = "127.0.0.1:0"
	defaultCaptureCapacity = 1000
	maxCaptureCapacity     = 1000000
)

// RuntimeOptions configures the simulator runtime. Zero values are replaced
// with loopback-safe defaults.
type RuntimeOptions struct {
	HTTPAddr        string
	CaptureCapacity int
	Now             func() time.Time
	Logger          *slog.Logger
}

// Runtime owns the /_sim HTTP service, the manifest and the shared capture
// buffer. Modules (the MQTT broker now, Modbus/ChirpStack later) register
// their routes, endpoints, manifest sections and control actions before
// Start is called.
type Runtime struct {
	mu        sync.Mutex
	mux       *http.ServeMux
	now       func() time.Time
	logger    *slog.Logger
	capture   *CaptureBuffer
	controls  *controlRegistry
	endpoints map[string]string
	sections  map[string]any
	modules   []string

	httpAddr string
	listener net.Listener
	server   *http.Server
	baseURL  string
	httpHost string
	httpPort int
	started  bool
	closed   bool
}

// NewRuntime builds a runtime. It does not bind ports until Start.
func NewRuntime(opts RuntimeOptions) (*Runtime, error) {
	addr := strings.TrimSpace(opts.HTTPAddr)
	if addr == "" {
		addr = defaultHTTPAddr
	}
	capacity := opts.CaptureCapacity
	if capacity == 0 {
		capacity = defaultCaptureCapacity
	}
	if capacity < 0 || capacity > maxCaptureCapacity {
		return nil, fmt.Errorf("capture capacity must be between 1 and %d", maxCaptureCapacity)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	r := &Runtime{
		mux:       http.NewServeMux(),
		now:       now,
		logger:    logger,
		capture:   NewCaptureBuffer(capacity, now),
		controls:  newControlRegistry(),
		endpoints: make(map[string]string),
		sections:  make(map[string]any),
		modules:   make([]string, 0, 1),
		httpAddr:  addr,
	}
	r.mux.HandleFunc("POST /_sim/control", r.handleControl)
	return r, nil
}

// Capture returns the shared uplink capture buffer.
func (r *Runtime) Capture() *CaptureBuffer { return r.capture }

// Now returns the runtime's injected clock.
func (r *Runtime) Now() time.Time { return r.now() }

// Handle registers an HTTP route. All routes must be registered before Start.
func (r *Runtime) Handle(pattern string, h http.HandlerFunc) {
	if pattern == "" || h == nil {
		panic("sim: route pattern and handler are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		panic("sim: routes must be registered before Start")
	}
	r.mux.HandleFunc(pattern, h)
}

// SetEndpoint adds an entry to manifest.endpoints. Names must be unique.
func (r *Runtime) SetEndpoint(name, path string) error {
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	if name == "" || path == "" {
		return errors.New("endpoint name and path are required")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("endpoint %q path must start with /", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.endpoints[name]; ok {
		return fmt.Errorf("endpoint %q is already registered", name)
	}
	r.endpoints[name] = path
	return nil
}

// SetSection adds a top-level manifest section and records the section name
// as a module. Modules may not replace manifest-owned sections.
func (r *Runtime) SetSection(name string, v any) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("section name is required")
	}
	if _, ok := reservedSections[name]; ok {
		return fmt.Errorf("section %q is reserved", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sections[name]; ok {
		return fmt.Errorf("section %q is already registered", name)
	}
	r.sections[name] = v
	if !slices.Contains(r.modules, name) {
		r.modules = append(r.modules, name)
	}
	return nil
}

// RegisterControl registers a control action. module "" is a global action;
// non-empty modules let 06a/12b add their own actions without name clashes.
func (r *Runtime) RegisterControl(module, action string, fn ControlFunc) error {
	return r.controls.add(module, action, fn)
}

// Start binds the HTTP listener and begins serving /_sim routes.
func (r *Runtime) Start(_ context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return errors.New("sim: runtime is already started")
	}
	addr := r.httpAddr
	r.mu.Unlock()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	host, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		_ = ln.Close()
		return fmt.Errorf("http address %q: %w", ln.Addr().String(), err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		_ = ln.Close()
		return fmt.Errorf("http port %q: %w", portText, err)
	}
	server := &http.Server{
		Handler:           r.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	r.mu.Lock()
	r.listener = ln
	r.server = server
	r.baseURL = "http://" + ln.Addr().String()
	r.httpHost = host
	r.httpPort = port
	r.started = true
	r.mu.Unlock()

	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.logger.Error("sim http server stopped", "error", err)
		}
	}()
	return nil
}

// BaseURL is the actual HTTP base URL, available after Start.
func (r *Runtime) BaseURL() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.baseURL
}

// HTTPAddress is the actual host:port, available after Start.
func (r *Runtime) HTTPAddress() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.httpHost == "" {
		return ""
	}
	return net.JoinHostPort(r.httpHost, strconv.Itoa(r.httpPort))
}

// Close stops the HTTP server and releases the port.
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	server := r.server
	r.mu.Unlock()

	if server == nil {
		return nil
	}
	if err := server.Shutdown(ctx); err != nil {
		_ = server.Close()
		return err
	}
	return nil
}

// WriteManifest atomically writes the startup manifest. An empty path is a
// no-op, so --manifest remains optional.
func (r *Runtime) WriteManifest(path string) error {
	if path == "" {
		return nil
	}
	doc, err := r.manifestDoc()
	if err != nil {
		return err
	}
	return writeManifestAtomic(path, doc)
}

func (r *Runtime) manifestDoc() (map[string]any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started {
		return nil, errors.New("sim: runtime must be started before writing the manifest")
	}

	endpoints := make(map[string]string, len(r.endpoints))
	maps.Copy(endpoints, r.endpoints)
	modules := make([]string, len(r.modules))
	copy(modules, r.modules)
	sort.Strings(modules)

	doc := map[string]any{
		"schema_version": manifestSchemaVersion,
		"generated_at":   r.now().UTC().Format(time.RFC3339Nano),
		"pid":            os.Getpid(),
		"http": map[string]any{
			"base_url": r.baseURL,
			"host":     r.httpHost,
			"port":     r.httpPort,
		},
		"endpoints":       endpoints,
		"control_actions": r.controls.byModule(),
		"modules":         modules,
	}
	maps.Copy(doc, r.sections)
	return doc, nil
}
