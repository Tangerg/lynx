package protocol

// Runtime is the runtime's public surface: the union of every method group
// exposed over the wire.
type Runtime interface {
	Lifecycle
	Sessions
	Runs
	Items
	Plan
	RuntimeSubscription
	Workspace
	Skills
	Recipes
	AgentDocs
	MCP
	Hooks
	Approval
	Schedules
	Goals
	Codebase
	Providers
	Models
	Tools
	Knowledge
	AgentMemory
	Feedback
	UsageReports
}
