// Package scheduler runs phase-locked, per-link acquisition ticks on an
// injected clock. It serialises devices within a link and gives every device
// its own read deadline; device state, freshness and offline detection belong
// to the state component.
package scheduler
