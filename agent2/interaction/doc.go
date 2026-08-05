// Package interaction provides the model-directed execution Strategy for the
// Agent Framework.
//
// A Definition owns the serializable working context and bounded model/tool
// state machine. A Dispatcher owns chatclient and executable tool I/O. Both are
// bound into one agent Deployment; neither owns a Process lifecycle, product
// conversation history, persistence, pricing, approval policy, or UI records.
// Direct model calls remain available through package chatclient without
// constructing an Interaction or Engine.
package interaction
