// Package agent provides the Lynx Agent Framework execution kernel.
//
// Definitions own immutable behavior and create serializable Executions;
// Engine owns Process lifecycle, Signals, Effects, child composition, resource
// bounds, observation, and portable Process/tree snapshots. Strategy payloads
// remain opaque to the kernel, and persistence remains a caller responsibility.
package agent
