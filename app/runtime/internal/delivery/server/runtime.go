package server

// RuntimeServices is the accessor surface the protocol server needs from
// the runtime bundle. Defined here (consumer side) so the server depends
// on the narrow set of accessors it actually calls, not the concrete
// *internal/runtime.Runtime — which keeps the protocol layer free of an
// internal-package import and lets a future remote runtime (or a test
// fake) satisfy the surface without standing up the real bundle.
//
// *internal/runtime.Runtime satisfies this implicitly; the composition
// root (cmd/lyra) passes the concrete value where a RuntimeServices is
// expected.
type RuntimeServices interface {
	turnAccess
	sessionAccess
	transcriptAccess
	lifecycleAccess
	runSegmentAccess
	historyAccess
	interruptQueryAccess
	toolAccess
	knowledgeAccess
	approvalAccess
	scheduleAccess
	providerAccess
	mcpAccess
	workspaceCatalogAccess
	hookAccess
	modelRoleAccess
	codebaseAccess
	maintenanceAccess
}
