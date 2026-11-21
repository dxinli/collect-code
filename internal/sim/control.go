package sim

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// ControlRequest is passed to a control action. Body is the complete raw JSON
// request, so the action can decode its own fields with DisallowUnknownFields.
type ControlRequest struct {
	Module string
	Action string
	Body   json.RawMessage
}

// ControlFunc handles one control action. Return a *BadRequestError for a
// client error (HTTP 400); any other error becomes HTTP 500.
type ControlFunc func(ControlRequest) (any, error)

type controlKey struct {
	module string
	action string
}

type controlRegistry struct {
	mu      sync.Mutex
	actions map[controlKey]ControlFunc
}

func newControlRegistry() *controlRegistry {
	return &controlRegistry{actions: make(map[controlKey]ControlFunc)}
}

func (c *controlRegistry) add(module, action string, fn ControlFunc) error {
	if action == "" {
		return fmt.Errorf("control action must not be empty")
	}
	if fn == nil {
		return fmt.Errorf("control action %q has no handler", action)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := controlKey{module: module, action: action}
	if _, ok := c.actions[key]; ok {
		return fmt.Errorf("control action %q is already registered for module %q", action, module)
	}
	c.actions[key] = fn
	return nil
}

func (c *controlRegistry) get(module, action string) (ControlFunc, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn, ok := c.actions[controlKey{module: module, action: action}]
	return fn, ok
}

// byModule returns sorted action names grouped by module. Global actions
// (module "") are reported under "global".
func (c *controlRegistry) byModule() map[string][]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make(map[string][]string)
	for key := range c.actions {
		name := key.module
		if name == "" {
			name = "global"
		}
		out[name] = append(out[name], key.action)
	}
	for name := range out {
		sort.Strings(out[name])
	}
	return out
}

// BadRequestError marks an action error as caused by the request.
type BadRequestError struct {
	Message string
}

func (e *BadRequestError) Error() string { return e.Message }

// BadRequestf builds a BadRequestError for HTTP 400.
func BadRequestf(format string, args ...any) error {
	return &BadRequestError{Message: fmt.Sprintf(format, args...)}
}
