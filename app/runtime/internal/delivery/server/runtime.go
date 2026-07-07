package server

// RuntimePort is the inbound adapter's consumer-side port into the runtime
// application boundary. Defined here so the server depends on the narrow set of
// operations it calls, not the concrete *internal/runtime.Runtime — which keeps
// the protocol layer free of an internal-package import and lets a future
// remote runtime (or a test fake) satisfy the surface without standing up the
// real bundle.
//
// *internal/runtime.Runtime satisfies this implicitly; the composition
// root (cmd/lyra) passes the concrete value where a RuntimePort is
// expected.
type RuntimePort interface {
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
	mcpStatusAccess
	mcpToolCatalogAccess
	mcpConnectionAccess
	mcpRegistryAccess
	workspaceCatalogAccess
	hookAccess
	modelRoleAccess
	codebaseAccess
	maintenanceAccess
}
