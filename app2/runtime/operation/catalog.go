package operation

// The machine-readable projection of everything registered here is generated,
// never hand-written; CI reruns this and fails on a worktree diff, which is the
// only mechanism that notices when the code and the published contract disagree
// (contract §11.4 gate 1).
//
//go:generate go run ../cmd/contractgen -out ../contract

// contract is the runtime's method surface. It is built once, at package init,
// from method expressions — so it exists without a Runtime and a build-time tool
// can read the whole contract without standing one up.
//
// Registrations are grouped into files by domain, mirroring the wire method
// groups (API.md §7). Adding a method is one registration; there is no
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

// requires builds the common rule: the whole method needs these features.
func requires(features ...string) []CapabilityRule {
	return []CapabilityRule{{Requires: features}}
}
