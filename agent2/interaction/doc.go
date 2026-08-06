// Package interaction provides the model-directed execution Strategy for the
// Agent Framework.
//
// A Definition owns the serializable working context, bounded model/Tool state
// machine, and exact managed Delegate bindings. A Dispatcher owns chatclient
// and ordinary executable Tool I/O. Delegate child Processes are requested only
// by the Execution through Framework Effects. Both components are bound into
// one agent Deployment; neither owns a Process lifecycle, product conversation
// history, persistence, pricing, approval policy, or UI records. Direct model
// calls remain available through package chatclient without constructing an
// Interaction or Engine.
package interaction
