package dispatch

// The machine-readable projection of everything registered here is generated,
// never hand-written; CI reruns this and fails on a worktree diff, which is the
// only mechanism that notices when the code and the published contract disagree
// (contract §11.4 gate 1).
//
//go:generate go run github.com/Tangerg/lynx/app/runtime/cmd/contractgen -out ../../../contract -validators ../protocol -ts ../../../contract/typescript

import "github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"

// contract is the runtime's method surface. It is built once, at package init,
// from method expressions — so it exists without a Runtime and a build-time tool
// can read the whole contract without standing one up.
//
// Registrations are grouped by domain in contract_<domain>.go, mirroring the wire
// method groups (API.md §7). Adding a method is one registration; there is no
// second table, name constant, or replay list to update alongside it.
var contract = buildContract()

func buildContract() *Registry {
	registry := newRegistry()
	registerLifecycle(registry)
	registerSessions(registry)
	registerRuns(registry)
	registerInterrupts(registry)
	registerPlan(registry)
	registerItems(registry)
	registerWorkspace(registry)
	registerRuntimeSubscription(registry)
	registerSkills(registry)
	registerRecipes(registry)
	registerAgentDocs(registry)
	registerMCP(registry)
	registerHooks(registry)
	registerApproval(registry)
	registerSchedules(registry)
	registerGoals(registry)
	registerCodebase(registry)
	registerProviders(registry)
	registerModels(registry)
	registerTools(registry)
	registerUsage(registry)
	registerKnowledge(registry)
	registerAgentMemory(registry)
	registerFeedback(registry)
	return registry
}

// Notification method names the server sends downstream (API.md §7.8). They are
// not registered methods: nothing inbound routes to them, and a client never
// calls them.
const (
	NotificationRunEvent     = "notifications.run.event"
	NotificationRuntimeEvent = "notifications.runtime.event"
)

// stable is the stability every method carries today. Named so a future
// experimental method reads as a deliberate exception rather than a typo.
const stable = protocol.StabilityStable

// requires builds the common rule: the whole method needs these features.
func requires(features ...string) []CapabilityRule {
	return []CapabilityRule{{Requires: features}}
}
