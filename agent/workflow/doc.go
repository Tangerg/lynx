// Package workflow provides agent-builders for common multi-step
// LLM patterns: scatter-gather (parallel fan-out + consolidation),
// repeat-until (loop until acceptance), and the supporting
// [Feedback] / [History] / [ResultList] types.
//
// Each builder produces a [*core.Agent] you deploy on a [*runtime.Platform]
// and run via [Platform.RunAgent] or compose as a sub-agent through
// [runtime.AsChatTool] / [runtime.AsMCPTool]. The agents are normal
// GOAP-planned agents — no new runtime concepts; the workflow is
// expressed via action effects and computed conditions the planner
// already understands.
package workflow
