// Package interfaces freezes the kernel's two seams.
//
// The kernel reaches the outside world only through:
//
//   - Driver, Channel and the uplink/downlink adapters for protocol and
//     transport; and
//   - Store for persistence.
//
// Concrete implementations live outside this package: drivers, channels and
// adapters in their own packages, the production Store under
// internal/store/bbolt. Neither internal/core nor internal/drivers may import
// a concrete implementation; scripts/check-deps.sh enforces the boundary.
//
// Time is a separate seam: Clock is the only source of time for kernel and
// driver code, and business deadlines MUST be derived from it rather than
// from context.WithTimeout or package-level time functions.
package interfaces
