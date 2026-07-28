// Package agent is the standard façade for the Lynx Agent Framework. It
// exposes definition, deployment, execution, status, interaction, and
// suspension types used by the common lifecycle. These are aliases to their
// owning core/runtime/interaction types, not copied abstractions.
//
// Advanced protocols remain in focused sub-packages: custom planners,
// Blackboard implementations, event payloads, tool-loop internals,
// workflow builders, and provider/tool adapters are imported only when used.
package agent
