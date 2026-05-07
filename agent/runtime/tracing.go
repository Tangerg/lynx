package runtime

// Tracing span / attribute keys. Centralised so a typo at one call site
// is impossible and external listeners (Prometheus exporters, OTel
// dashboards) have a single source of truth for the schema. Keep these
// stable across releases — renaming an attribute breaks every dashboard
// keyed off it.
const (
	spanTick   = "lynx.agent.tick"
	spanAction = "lynx.agent.action"

	attrAgentName       = "lynx.agent.name"
	attrProcessID       = "lynx.agent.process_id"
	attrActionName      = "lynx.agent.action.name"
	attrActionStatus    = "lynx.agent.action.status"
	attrActionAttempts  = "lynx.agent.action.attempts"
	attrWorldStateSize  = "lynx.agent.world_state.size"
)
