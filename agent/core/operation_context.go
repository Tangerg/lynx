package core

// OperationContext is the read-only surface a Condition.Evaluate sees. It's
// kept small intentionally: a condition should not need a chat client, an
// LLM, or a publish channel to decide whether a fact holds. (Prompt-driven
// conditions plug in via PromptCondition, which carries its own client.)
type OperationContext struct {
	Process    Process
	Blackboard Blackboard
}
