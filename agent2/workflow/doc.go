// Package workflow provides deterministic orchestration of Framework-managed
// child Processes. A Workflow is an ordered sequence of sealed Stages; it is
// not a general in-process task graph, scheduler, journal, or node registry.
//
// Use this package when each delegated operation needs its own Process
// identity, snapshot, budget, capabilities, cancellation, and tree recovery.
// Ordinary in-process control flow belongs outside the Agent Framework.
package workflow
