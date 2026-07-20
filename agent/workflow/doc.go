// Package workflow provides agent-builders for common multi-step
// LLM patterns: scatter-gather (parallel fan-out + consolidation),
// repeat-until (loop until acceptance), team definition composition, and the supporting
// [Feedback] and [History] types.
//
// Each builder produces a [*core.Agent] you deploy on a [*runtime.Engine]
// and run via [Engine.Run] or compose as a sub-agent through
// [runtime.NewAgentTool] / [runtime.NewStandaloneAgentTool]. The agents are normal
// GOAP-planned agents — no new runtime concepts; the workflow is
// expressed via action effects and computed conditions the planner
// already understands.
package workflow
