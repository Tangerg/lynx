package contractcatalog

import (
	"reflect"
	"slices"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

// A wire enum is a named string type with a closed value set. Reflection can see
// that a field's type is RunStatus; it can never see that RunStatus has exactly
// two values, because a Go const block is not enumerable at runtime.
//
// So the set is declared here — by referencing the constants, never by repeating
// their text, so a value can only be renamed in one place — and TestWireEnumsAreComplete
// checks the declaration against the constants the package actually defines. Add
// a constant and forget this table and the test names it.
//
// Without the declaration every generated schema and TS type would say `string`
// where the runtime accepts three words, which is a published contract that
// permits frames the runtime rejects.
var wireEnums = map[reflect.Type][]string{
	reflect.TypeFor[protocol.AgentDocScope]():                     {string(protocol.AgentDocScopeCWD), string(protocol.AgentDocScopeProjectRoot), string(protocol.AgentDocScopeHome)},
	reflect.TypeFor[protocol.AgentMemoryOrigin]():                 {string(protocol.AgentMemoryOriginAuto), string(protocol.AgentMemoryOriginUser)},
	reflect.TypeFor[protocol.AgentMemoryReviewDecision]():         {string(protocol.AgentMemoryReviewApprove), string(protocol.AgentMemoryReviewReject)},
	reflect.TypeFor[protocol.AgentMemoryScope]():                  {string(protocol.AgentMemoryScopeProject), string(protocol.AgentMemoryScopeUser)},
	reflect.TypeFor[protocol.AgentMemoryStatus]():                 {string(protocol.AgentMemoryStatusActive), string(protocol.AgentMemoryStatusPending)},
	reflect.TypeFor[protocol.ApprovalDecision]():                  {string(protocol.ApprovalApprove), string(protocol.ApprovalDeny)},
	reflect.TypeFor[protocol.ApprovalMode]():                      {string(protocol.ApprovalModeSafe), string(protocol.ApprovalModeBalanced), string(protocol.ApprovalModeYolo)},
	reflect.TypeFor[protocol.ApprovalRisk]():                      {string(protocol.ApprovalRiskLow), string(protocol.ApprovalRiskMedium), string(protocol.ApprovalRiskHigh)},
	reflect.TypeFor[protocol.ApprovalRuleDecision]():              {string(protocol.ApprovalRuleDecisionAllow), string(protocol.ApprovalRuleDecisionDeny)},
	reflect.TypeFor[protocol.ApprovalRuleScope]():                 {string(protocol.ApprovalRuleScopeSession), string(protocol.ApprovalRuleScopeProject), string(protocol.ApprovalRuleScopeGlobal)},
	reflect.TypeFor[protocol.ArtifactOutcomeType]():               {string(protocol.ArtifactOutcomeCompleted), string(protocol.ArtifactOutcomeTimedOut), string(protocol.ArtifactOutcomeFailed), string(protocol.ArtifactOutcomeMaxSteps), string(protocol.ArtifactOutcomeMaxBudget), string(protocol.ArtifactOutcomeCanceled), string(protocol.ArtifactOutcomeLost)},
	reflect.TypeFor[protocol.ArtifactProblemType]():               {string(protocol.ArtifactProblemInternalError), string(protocol.ArtifactProblemRunLost), string(protocol.ArtifactProblemAgentStuck), string(protocol.ArtifactProblemRateLimited), string(protocol.ArtifactProblemInvalidAPIKey), string(protocol.ArtifactProblemTimeout), string(protocol.ArtifactProblemProviderUnavailable), string(protocol.ArtifactProblemProviderRejected), string(protocol.ArtifactProblemDeniedByUser), string(protocol.ArtifactProblemToolFailed), string(protocol.ArtifactProblemChildRunCanceled), string(protocol.ArtifactProblemToolCanceled)},
	reflect.TypeFor[protocol.ContentBlockType]():                  {string(protocol.ContentBlockText), string(protocol.ContentBlockImage)},
	reflect.TypeFor[protocol.SuppressibleRunEventType]():          {string(protocol.SuppressibleRunSegmentProgress), string(protocol.SuppressibleRunItemDelta)},
	reflect.TypeFor[protocol.DiffFormat]():                        {string(protocol.DiffFormatRows), string(protocol.DiffFormatRaw)},
	reflect.TypeFor[protocol.DiffMode]():                          {string(protocol.DiffModeWorktree), string(protocol.DiffModeBase)},
	reflect.TypeFor[protocol.DiffRowType]():                       {string(protocol.DiffRowHunk), string(protocol.DiffRowContext), string(protocol.DiffRowAdded), string(protocol.DiffRowDeleted)},
	reflect.TypeFor[protocol.ExportFormat]():                      {string(protocol.ExportFormatMarkdown), string(protocol.ExportFormatJSON)},
	reflect.TypeFor[protocol.FeedbackRating]():                    {string(protocol.FeedbackPositive), string(protocol.FeedbackNegative)},
	reflect.TypeFor[protocol.FileEntryType]():                     {string(protocol.FileEntryFile), string(protocol.FileEntryDir), string(protocol.FileEntrySymlink)},
	reflect.TypeFor[protocol.FileStatus]():                        {string(protocol.FileStatusAdded), string(protocol.FileStatusModified), string(protocol.FileStatusDeleted), string(protocol.FileStatusRenamed), string(protocol.FileStatusUntracked)},
	reflect.TypeFor[protocol.CapabilityRequirementType]():         {string(protocol.RequirementFeature), string(protocol.RequirementInterruptType), string(protocol.RequirementRuntimeTopic)},
	reflect.TypeFor[protocol.CancelRunResponseType]():             {string(protocol.CancelRunRoot), string(protocol.CancelRunChild)},
	reflect.TypeFor[protocol.RecoveryAction]():                    {string(protocol.RecoveryRefetch), string(protocol.RecoveryColdRecover), string(protocol.RecoveryResubscribe), string(protocol.RecoveryReauthenticate), string(protocol.RecoveryWaitRetryAfter), string(protocol.RecoveryPromptUser), string(protocol.RecoveryStop)},
	reflect.TypeFor[protocol.GoalStatus]():                        {string(protocol.GoalActive), string(protocol.GoalPaused), string(protocol.GoalBlocked), string(protocol.GoalCompleting)},
	reflect.TypeFor[protocol.GoalReasonCode]():                    {string(protocol.GoalReasonStoppedByUser), string(protocol.GoalReasonRuntimeRestarted), string(protocol.GoalReasonRunStartFailed), string(protocol.GoalReasonAwaitingInput), string(protocol.GoalReasonTerminalOutcomeMissing), string(protocol.GoalReasonRunNotCompleted), string(protocol.GoalReasonRunBudgetReached), string(protocol.GoalReasonCostBudgetReached), string(protocol.GoalReasonStepBudgetReached), string(protocol.GoalReasonBlockedByModel)},
	reflect.TypeFor[protocol.HookEvent]():                         {string(protocol.HookEventPreToolUse), string(protocol.HookEventPostToolUse), string(protocol.HookEventUserPromptSubmit), string(protocol.HookEventSessionStart), string(protocol.HookEventSubagentStart), string(protocol.HookEventSubagentStop), string(protocol.HookEventPreCompact), string(protocol.HookEventStop), string(protocol.HookEventNotification)},
	reflect.TypeFor[protocol.HookScope]():                         {string(protocol.HookScopeGlobal), string(protocol.HookScopeProject)},
	reflect.TypeFor[protocol.InterruptResponseType]():             {string(protocol.InterruptResponseApproval), string(protocol.InterruptResponseAnswer)},
	reflect.TypeFor[protocol.InterruptType]():                     {string(protocol.InterruptApproval), string(protocol.InterruptQuestion)},
	reflect.TypeFor[protocol.ItemDeltaType]():                     {string(protocol.DeltaContent), string(protocol.DeltaReasoning), string(protocol.DeltaToolArguments), string(protocol.DeltaToolOutput)},
	reflect.TypeFor[protocol.ItemOrder]():                         {string(protocol.ItemOrderAsc), string(protocol.ItemOrderDesc)},
	reflect.TypeFor[protocol.ItemScopeType]():                     {string(protocol.ItemScopeSession), string(protocol.ItemScopeRun)},
	reflect.TypeFor[protocol.ItemStatus]():                        {string(protocol.ItemStatusRunning), string(protocol.ItemStatusCompleted), string(protocol.ItemStatusIncomplete)},
	reflect.TypeFor[protocol.ItemType]():                          {string(protocol.ItemTypeUserMessage), string(protocol.ItemTypeAgentMessage), string(protocol.ItemTypeReasoning), string(protocol.ItemTypeQuestion), string(protocol.ItemTypeToolCall), string(protocol.ItemTypeCompaction)},
	reflect.TypeFor[protocol.MessagePhase]():                      {string(protocol.MessagePhaseCommentary), string(protocol.MessagePhaseFinalAnswer)},
	reflect.TypeFor[protocol.MCPSecretChangeType]():               {string(protocol.MCPSecretSet), string(protocol.MCPSecretClear)},
	reflect.TypeFor[protocol.MCPAuthorizationAttemptStatusType](): {string(protocol.MCPAuthorizationAttemptPending), string(protocol.MCPAuthorizationAttemptSucceeded), string(protocol.MCPAuthorizationAttemptFailed), string(protocol.MCPAuthorizationAttemptCanceled)},
	reflect.TypeFor[protocol.MCPServerStateType]():                {string(protocol.MCPServerDisabled), string(protocol.MCPServerDisconnected), string(protocol.MCPServerConnecting), string(protocol.MCPServerConnected), string(protocol.MCPServerFailed), string(protocol.MCPServerNeedsAuth)},
	reflect.TypeFor[protocol.MCPTransport]():                      {string(protocol.MCPTransportStdio), string(protocol.MCPTransportStreamableHTTP)},
	reflect.TypeFor[protocol.KnowledgeScope]():                    {string(protocol.KnowledgeScopeCWD), string(protocol.KnowledgeScopeProjectRoot), string(protocol.KnowledgeScopeHome)},
	reflect.TypeFor[protocol.Modality]():                          {string(protocol.ModalityText), string(protocol.ModalityImage), string(protocol.ModalityAudio), string(protocol.ModalityVideo), string(protocol.ModalityPDF)},
	reflect.TypeFor[protocol.ProviderKeySource]():                 {string(protocol.ProviderKeySourceStored), string(protocol.ProviderKeySourceEnv)},
	reflect.TypeFor[protocol.QuestionFieldType]():                 {string(protocol.QuestionFieldText), string(protocol.QuestionFieldChoice)},
	reflect.TypeFor[protocol.ProviderConfigChangeType]():          {string(protocol.ProviderConfigSet), string(protocol.ProviderConfigClear)},
	reflect.TypeFor[protocol.RecipeScope]():                       {string(protocol.RecipeScopeProject), string(protocol.RecipeScopeGlobal)},
	reflect.TypeFor[protocol.RememberScopeKind]():                 {string(protocol.RememberSession), string(protocol.RememberProject), string(protocol.RememberGlobal)},
	reflect.TypeFor[protocol.RestoreType]():                       {string(protocol.RestoreHistory), string(protocol.RestoreFiles), string(protocol.RestoreBoth)},
	reflect.TypeFor[protocol.RunOutcomeType]():                    {string(protocol.OutcomeCompleted), string(protocol.OutcomeTimedOut), string(protocol.OutcomeFailed), string(protocol.OutcomeMaxSteps), string(protocol.OutcomeMaxBudget), string(protocol.OutcomeCanceled), string(protocol.OutcomeLost)},
	reflect.TypeFor[protocol.RunProtocolFeature]():                runProtocolFeatureValues(),
	reflect.TypeFor[protocol.RunStatus]():                         {string(protocol.RunStatusRunning), string(protocol.RunStatusWaiting), string(protocol.RunStatusFinished)},
	reflect.TypeFor[protocol.ScheduleWorkspaceMode]():             {string(protocol.ScheduleWorkspaceDefault)},
	reflect.TypeFor[protocol.SegmentOutcomeType]():                {string(protocol.SegmentInterrupt), string(protocol.SegmentSuspended), string(protocol.SegmentCompleted), string(protocol.SegmentTimedOut), string(protocol.SegmentFailed), string(protocol.SegmentMaxSteps), string(protocol.SegmentMaxBudget), string(protocol.SegmentCanceled), string(protocol.SegmentLost)},
	reflect.TypeFor[protocol.SafetyClass]():                       {string(protocol.SafetyClassSafe), string(protocol.SafetyClassWrite), string(protocol.SafetyClassExec), string(protocol.SafetyClassNetwork)},
	reflect.TypeFor[protocol.SessionStatus]():                     {string(protocol.SessionStatusRunning), string(protocol.SessionStatusWaiting), string(protocol.SessionStatusIdle)},
	reflect.TypeFor[protocol.SkillLifecycle]():                    {string(protocol.SkillLifecycleActive), string(protocol.SkillLifecycleArchived)},
	reflect.TypeFor[protocol.SkillScope]():                        {string(protocol.SkillScopeProject), string(protocol.SkillScopeUser)},
	reflect.TypeFor[protocol.SkillProposalOrigin]():               {string(protocol.SkillProposalOriginRequested), string(protocol.SkillProposalOriginMined)},
	reflect.TypeFor[protocol.StreamEventType]():                   {string(protocol.StreamSegmentStarted), string(protocol.StreamSegmentProgress), string(protocol.StreamSegmentFinished), string(protocol.StreamItemStarted), string(protocol.StreamItemDelta), string(protocol.StreamItemCompleted), string(protocol.StreamPlanUpdated)},
	reflect.TypeFor[protocol.PlanStatus]():                        {string(protocol.PlanStatusPending), string(protocol.PlanStatusInProgress), string(protocol.PlanStatusCompleted)},
	reflect.TypeFor[protocol.WorkspaceAvailability]():             {string(protocol.WorkspaceAvailable), string(protocol.WorkspaceMissing)},
	reflect.TypeFor[protocol.RuntimeEventType]():                  runtimeEventValues(),
	reflect.TypeFor[protocol.RuntimeTopic]():                      runtimeTopicValues(),
	reflect.TypeFor[protocol.RunReplayScope]():                    {string(protocol.ReplayScopeRuntimeInstanceRootSegment)},
}

// EnumValues reports the closed value set of a wire enum type, and false when t is
// not one. Callers are build-time artifact generators and the wire validator;
// both read the set, neither owns it.
func EnumValues(t reflect.Type) ([]string, bool) {
	values, ok := wireEnums[t]
	return slices.Clone(values), ok
}

func runProtocolFeatureValues() []string {
	var values []string
	for _, feature := range protocol.Features() {
		if feature.RequiredByRunProtocol {
			values = append(values, feature.Key)
		}
	}
	return values
}

// runtimeEventValues is the event vocabulary: every topic, plus resync. Derived from
// the topic list rather than written twice — that duplication is exactly what having
// one set of strings prevents.
func runtimeEventValues() []string {
	out := make([]string, 0, len(protocol.RuntimeTopics())+1)
	out = append(out, runtimeTopicValues()...)
	return append(out, string(protocol.RuntimeResync))
}

func runtimeTopicValues() []string {
	topics := protocol.RuntimeTopics()
	out := make([]string, 0, len(topics))
	for _, topic := range topics {
		out = append(out, string(topic))
	}
	return out
}
