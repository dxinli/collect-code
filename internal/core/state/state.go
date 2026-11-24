// Package state is the kernel device-state component.
//
// It consumes one read result per poll and one driver event per push
// observation, computes the final quality of every point, advances the last
// success time only on fresh observations, and decides device offline and
// recovery transitions. Drivers may report a bad initial quality; this
// component never upgrades a bad observation. Time enters only through the
// injected Clock.
package state

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/dxinli/collect-code/internal/core/scheduler"
	"github.com/dxinli/collect-code/internal/interfaces"
)

// defaultSweepEvery is the offline-check resolution used when Config leaves
// SweepEvery unset. It is an internal detection cadence, not a device
// parameter: every configurable period is at least one second.
const defaultSweepEvery = time.Second

// Config is the resolved per-device plan. The assembly layer resolves
// point.stale_after_s ?? timing.freshness_window_s and the effective period
// before constructing a Tracker; this component never reads YAML.
type Config struct {
	// OfflineAfter is timing.offline_after_intervals (>= 1).
	OfflineAfter int
	// SweepEvery is the offline-check resolution. Zero means one second.
	SweepEvery time.Duration
	// Devices is in configuration order.
	Devices []Device
}

// Device is one monitored device.
type Device struct {
	ID string
	// Period is the effective poll period for a pollable device, or the
	// expected uplink interval for a push device. It is the unit used by
	// the consecutive-periods offline rule.
	Period time.Duration
	Points []Point
}

// Point is one quality-bearing point.
type Point struct {
	Key        string
	Unit       string
	StaleAfter time.Duration // effective threshold, already resolved
}

// OnlineChange reports one device online-state transition.
type OnlineChange struct {
	Device string
	Online bool
	At     time.Time
}

// Sink receives wire-ready state changes. Implementations must not block and
// must not call back into Tracker: they should enqueue. Calls for one device
// preserve state-machine order because Tracker emits while holding its lock.
type Sink interface {
	// PublishProperties publishes changed properties (ReportDelta) or a full
	// snapshot (ReportSnapshot). online is the device state after any
	// transition in the same step.
	PublishProperties(device string, mode interfaces.ReportMode, online bool, props []interfaces.Property)
	// PublishOnline reports a single online-state transition.
	PublishOnline(OnlineChange)
}

// DeviceSnapshot is a read-only view of one device's state.
type DeviceSnapshot struct {
	Device        string
	Online        bool
	LastSuccessAt time.Time
	Properties    []interfaces.Property
}

// Tracker is the kernel device-state machine. It is safe for concurrent use.
type Tracker struct {
	clock        interfaces.Clock
	sink         Sink
	offlineAfter int
	sweepEvery   time.Duration
	order        []string
	devices      map[string]*deviceState

	mu sync.Mutex

	runMu sync.Mutex
	ran   bool
}

type deviceState struct {
	id          string
	period      time.Duration
	points      []Point
	pointIndex  map[string]int
	values      []pointValue
	online      bool
	lastSuccess time.Time
}

type pointValue struct {
	has   bool
	dirty bool
	prop  interfaces.Property
}

// New validates cfg and returns a Tracker with every device online and its
// last success time anchored at clock.Now(). A nil logger is not part of the
// state machine; this component has no logging side effects.
func New(clock interfaces.Clock, cfg Config, sink Sink) (*Tracker, error) {
	if isNil(clock) {
		return nil, errors.New("state: clock is required")
	}
	if isNil(sink) {
		return nil, errors.New("state: sink is required")
	}
	if cfg.OfflineAfter < 1 {
		return nil, fmt.Errorf("state: offline after must be >= 1 (got %d)", cfg.OfflineAfter)
	}
	if cfg.SweepEvery < 0 {
		return nil, fmt.Errorf("state: sweep interval must be >= 0 (got %s)", cfg.SweepEvery)
	}
	sweepEvery := cfg.SweepEvery
	if sweepEvery == 0 {
		sweepEvery = defaultSweepEvery
	}

	t := &Tracker{
		clock:        clock,
		sink:         sink,
		offlineAfter: cfg.OfflineAfter,
		sweepEvery:   sweepEvery,
		devices:      make(map[string]*deviceState, len(cfg.Devices)),
	}
	now := clock.Now()
	for i, dev := range cfg.Devices {
		if dev.ID == "" {
			return nil, fmt.Errorf("state: device %d has empty id", i)
		}
		if _, exists := t.devices[dev.ID]; exists {
			return nil, fmt.Errorf("state: duplicate device id %q", dev.ID)
		}
		if dev.Period <= 0 {
			return nil, fmt.Errorf("state: device %q period must be positive", dev.ID)
		}
		if len(dev.Points) == 0 {
			return nil, fmt.Errorf("state: device %q has no points", dev.ID)
		}
		ds := &deviceState{
			id:          dev.ID,
			period:      dev.Period,
			points:      append([]Point(nil), dev.Points...),
			pointIndex:  make(map[string]int, len(dev.Points)),
			values:      make([]pointValue, len(dev.Points)),
			online:      true,
			lastSuccess: now,
		}
		for j, point := range dev.Points {
			if point.Key == "" {
				return nil, fmt.Errorf("state: device %q has an empty point key", dev.ID)
			}
			if _, exists := ds.pointIndex[point.Key]; exists {
				return nil, fmt.Errorf("state: device %q has duplicate point %q", dev.ID, point.Key)
			}
			if point.StaleAfter <= 0 {
				return nil, fmt.Errorf("state: device %q point %q stale threshold must be positive", dev.ID, point.Key)
			}
			ds.pointIndex[point.Key] = j
			ds.values[j].prop = interfaces.Property{Key: point.Key, Unit: point.Unit}
		}
		t.devices[dev.ID] = ds
		t.order = append(t.order, dev.ID)
	}
	return t, nil
}

// ObserveRead implements scheduler.Observer. It applies one device read and
// may emit property or online changes.
func (t *Tracker) ObserveRead(result scheduler.ReadResult) {
	t.mu.Lock()
	defer t.mu.Unlock()

	dev := t.devices[result.Device]
	if dev == nil {
		return
	}
	now := t.clock.Now()
	fresh := t.applyRead(dev, now, result)
	t.finish(dev, now, fresh)
}

// ObserveTick implements scheduler.Observer. Tick observations do not change
// device state; offline detection runs on its own sweep.
func (t *Tracker) ObserveTick(scheduler.TickInfo) {}

// ObserveEvent applies one driver push event. Only uplink events carry fresh
// observations; acknowledgements and driver status never change properties or
// advance the last success time.
func (t *Tracker) ObserveEvent(event interfaces.DriverEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	dev := t.devices[event.Device]
	if dev == nil {
		return
	}
	now := t.clock.Now()
	fresh := false
	switch event.Kind {
	case interfaces.EventUplink:
		if len(event.Sample) == 0 {
			return
		}
		fresh = t.applySamples(dev, now, event.Sample, false)
	case interfaces.EventTxAck, interfaces.EventAck, interfaces.EventDeviceStatus, interfaces.EventDriverError:
		// No property change and no last-success advancement.
	default:
		return
	}
	t.finish(dev, now, fresh)
}

// Sweep evaluates the offline rule for every device at clock.Now().
func (t *Tracker) Sweep() {
	now := t.clock.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range t.order {
		t.finish(t.devices[id], now, false)
	}
}

// Run executes periodic offline sweeps until ctx is done. It may be called
// only once.
func (t *Tracker) Run(ctx context.Context) error {
	t.runMu.Lock()
	if t.ran {
		t.runMu.Unlock()
		return errors.New("state: already run")
	}
	t.ran = true
	t.runMu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	ticker := t.clock.NewTicker(t.sweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C():
			t.Sweep()
		}
	}
}

// Snapshot returns a copy of one device's state.
func (t *Tracker) Snapshot(device string) (DeviceSnapshot, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	dev := t.devices[device]
	if dev == nil {
		return DeviceSnapshot{}, false
	}
	return t.snapshot(dev), true
}

// Snapshots returns copies of every device state in configuration order.
func (t *Tracker) Snapshots() []DeviceSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]DeviceSnapshot, 0, len(t.order))
	for _, id := range t.order {
		out = append(out, t.snapshot(t.devices[id]))
	}
	return out
}

func (t *Tracker) applyRead(dev *deviceState, now time.Time, result scheduler.ReadResult) bool {
	switch result.Outcome {
	case interfaces.OutcomeUnsupported:
		return false
	case interfaces.OutcomeFailed:
		for i := range dev.points {
			t.markBad(dev, i, nil)
		}
		return false
	case interfaces.OutcomeOK:
		if len(result.Samples) == 0 {
			return false
		}
		return t.applySamples(dev, now, result.Samples, true)
	case interfaces.OutcomePartial:
		return t.applySamples(dev, now, result.Samples, true)
	default:
		return false
	}
}

// applySamples classifies the supplied samples. missingIsBad marks configured
// points absent from a poll read; uplink events may carry a subset and must
// leave the other points untouched.
func (t *Tracker) applySamples(dev *deviceState, now time.Time, samples []interfaces.Sample, missingIsBad bool) bool {
	byKey := make(map[string]interfaces.Sample, len(samples))
	for _, sample := range samples {
		if _, known := dev.pointIndex[sample.PointKey]; known {
			byKey[sample.PointKey] = sample
		}
	}

	fresh := false
	var newest time.Time
	for i, point := range dev.points {
		sample, ok := byKey[point.Key]
		if !ok {
			if missingIsBad {
				t.markBad(dev, i, nil)
			}
			continue
		}
		if sample.Quality == interfaces.QualityBad || sample.Err != "" || sample.At.IsZero() {
			t.markBad(dev, i, &sample)
			continue
		}
		if now.Sub(sample.At) > point.StaleAfter {
			t.markStale(dev, i, &sample)
			continue
		}
		t.markGood(dev, i, &sample)
		if !fresh || sample.At.After(newest) {
			newest = sample.At
		}
		fresh = true
	}
	if fresh && newest.After(dev.lastSuccess) {
		dev.lastSuccess = newest
	}
	return fresh
}

func (t *Tracker) markGood(dev *deviceState, index int, sample *interfaces.Sample) {
	pv := &dev.values[index]
	if pv.has && sample.At.Before(pv.prop.TS) {
		return
	}
	point := dev.points[index]
	pv.update(interfaces.Property{
		Key:     point.Key,
		Value:   sample.Value,
		Unit:    point.Unit,
		Quality: interfaces.QualityGood,
		TS:      sample.At,
	})
}

func (t *Tracker) markStale(dev *deviceState, index int, sample *interfaces.Sample) {
	pv := &dev.values[index]
	if pv.has {
		if sample.At.Before(pv.prop.TS) {
			return
		}
		// A valid but expired observation still does not move the stored
		// value or sample time forward.
		pv.setQuality(interfaces.QualityStale)
		return
	}
	point := dev.points[index]
	pv.update(interfaces.Property{
		Key:     point.Key,
		Value:   sample.Value,
		Unit:    point.Unit,
		Quality: interfaces.QualityStale,
		TS:      sample.At,
	})
}

func (t *Tracker) markBad(dev *deviceState, index int, sample *interfaces.Sample) {
	pv := &dev.values[index]
	if pv.has {
		pv.setQuality(interfaces.QualityBad)
		return
	}
	if sample == nil || sample.At.IsZero() || sample.Value == nil {
		return
	}
	point := dev.points[index]
	pv.update(interfaces.Property{
		Key:     point.Key,
		Value:   sample.Value,
		Unit:    point.Unit,
		Quality: interfaces.QualityBad,
		TS:      sample.At,
	})
}

// finish applies the online rule, then emits the resulting changes. It is
// called with the state lock held.
func (t *Tracker) finish(dev *deviceState, now time.Time, fresh bool) {
	offline := now.Sub(dev.lastSuccess) > time.Duration(t.offlineAfter)*dev.period
	switch {
	case dev.online && offline:
		dev.online = false
		t.markOffline(dev)
		t.sink.PublishOnline(OnlineChange{Device: dev.id, Online: false, At: now})
		t.publishDirty(dev, interfaces.ReportDelta)
	case !dev.online && fresh && !offline:
		dev.online = true
		t.sink.PublishOnline(OnlineChange{Device: dev.id, Online: true, At: now})
		t.clearDirty(dev)
		props := t.properties(dev)
		if len(props) > 0 {
			t.sink.PublishProperties(dev.id, interfaces.ReportSnapshot, true, props)
		}
	case dev.online:
		t.publishDirty(dev, interfaces.ReportDelta)
	default:
		// Already offline and not recovered: suppress property output.
		t.clearDirty(dev)
	}
}

func (t *Tracker) markOffline(dev *deviceState) {
	for i := range dev.points {
		pv := &dev.values[i]
		if pv.has && pv.prop.Quality == interfaces.QualityGood {
			pv.setQuality(interfaces.QualityStale)
		}
	}
}

func (t *Tracker) publishDirty(dev *deviceState, mode interfaces.ReportMode) {
	changed := make([]interfaces.Property, 0, len(dev.points))
	for i := range dev.points {
		pv := &dev.values[i]
		if !pv.dirty {
			continue
		}
		changed = append(changed, pv.prop)
		pv.dirty = false
	}
	if len(changed) == 0 {
		return
	}
	t.sink.PublishProperties(dev.id, mode, dev.online, changed)
}

func (t *Tracker) clearDirty(dev *deviceState) {
	for i := range dev.values {
		dev.values[i].dirty = false
	}
}

func (t *Tracker) properties(dev *deviceState) []interfaces.Property {
	out := make([]interfaces.Property, 0, len(dev.points))
	for i := range dev.points {
		pv := &dev.values[i]
		if pv.has {
			out = append(out, pv.prop)
		}
	}
	return out
}

func (t *Tracker) snapshot(dev *deviceState) DeviceSnapshot {
	return DeviceSnapshot{
		Device:        dev.id,
		Online:        dev.online,
		LastSuccessAt: dev.lastSuccess,
		Properties:    t.properties(dev),
	}
}

func (pv *pointValue) update(prop interfaces.Property) {
	if pv.has && propertyEqual(pv.prop, prop) {
		return
	}
	pv.has = true
	pv.prop = prop
	pv.dirty = true
}

func (pv *pointValue) setQuality(quality interfaces.Quality) {
	if !pv.has || pv.prop.Quality == quality {
		return
	}
	pv.prop.Quality = quality
	pv.dirty = true
}

func propertyEqual(a, b interfaces.Property) bool {
	return a.Key == b.Key &&
		a.Unit == b.Unit &&
		a.Quality == b.Quality &&
		a.TS.Equal(b.TS) &&
		reflect.DeepEqual(a.Value, b.Value)
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
