// Package interaction provides the model-directed execution Strategy for the
// Agent Framework.
//
// A Definition owns the serializable working context, bounded model/Tool state
// machine, exact managed Delegate bindings, typed Delegate Artifacts, and an
// optional pure completion validator. A Dispatcher owns chatclient and ordinary
// executable Tool I/O. Delegate child Processes are requested only by the
// Execution through Framework Effects. Both components are bound into one agent
// Deployment; neither owns a Process lifecycle, product conversation history,
// persistence, an application artifact store, pricing, approval policy, or UI
// records. Direct model calls remain available through package chatclient
// without constructing an Interaction or Engine.
package interaction
