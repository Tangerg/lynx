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
	turnStartAccess
	turnStreamAccess
	turnSteeringAccess
	turnInterruptPolicyAccess
	sessionCatalogAccess
	sessionCreationAccess
	sessionDeletionAccess
	sessionUpdateAccess
	sessionDefaultModelAccess
	transcriptContentAccess
	transcriptRunAccess
	runSlotAdmissionAccess
	workingTreeRunAdmissionAccess
	sessionMutationSlotAccess
	workingTreeMutationAccess
	runResumeAccess
	runCancellationAccess
	sessionRollbackAccess
	sessionForkAccess
	sessionRestoreAccess
	runSegmentAccess
	historyAccess
	interruptQueryAccess
	toolCatalogAccess
	toolInvocationAccess
	memoryAvailabilityAccess
	memoryStoreAccess
	approvalModeAccess
	approvalRuleAccess
	scheduleCatalogAccess
	scheduleMutationAccess
	scheduleRunRecorderAccess
	scheduleWorkerAccess
	providerRegistryCatalogAccess
	providerRegistryMutationAccess
	providerRegistryProbeAccess
	providerCatalogAccess
	providerDefaultAccess
	mcpStatusAccess
	mcpToolCatalogAccess
	mcpConnectionAccess
	mcpRegistryCatalogAccess
	mcpRegistryMutationAccess
	mcpRegistryProbeAccess
	skillCatalogAccess
	recipeCatalogAccess
	hookInspectionAccess
	hookTrustAccess
	utilityRoleAccess
	embeddingRoleAccess
	codebaseAvailabilityAccess
	codebaseSearchAccess
	codebaseStatusAccess
	codebaseReindexAccess
	maintenanceAccess
}
