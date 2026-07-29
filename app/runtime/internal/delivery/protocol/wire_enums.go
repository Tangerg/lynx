package protocol

import "reflect"

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
	reflect.TypeFor[AgentDocScope]():             {string(AgentDocScopeCwd), string(AgentDocScopeProjectRoot), string(AgentDocScopeHome)},
	reflect.TypeFor[AgentMemoryOrigin]():         {string(AgentMemoryOriginAuto), string(AgentMemoryOriginUser)},
	reflect.TypeFor[AgentMemoryReviewDecision](): {string(AgentMemoryReviewApprove), string(AgentMemoryReviewReject)},
	reflect.TypeFor[AgentMemoryScope]():          {string(AgentMemoryScopeProject), string(AgentMemoryScopeUser)},
	reflect.TypeFor[AgentMemoryStatus]():         {string(AgentMemoryStatusActive), string(AgentMemoryStatusPending)},
	reflect.TypeFor[ApprovalDecision]():          {string(ApprovalApprove), string(ApprovalDeny)},
	reflect.TypeFor[ApprovalMode]():              {string(ApprovalModeSafe), string(ApprovalModeBalanced), string(ApprovalModeYolo), string(ApprovalModePlan)},
	reflect.TypeFor[ApprovalRisk]():              {string(ApprovalRiskLow), string(ApprovalRiskMedium), string(ApprovalRiskHigh)},
	reflect.TypeFor[ApprovalRuleDecision]():      {string(ApprovalRuleDecisionAllow), string(ApprovalRuleDecisionDeny)},
	reflect.TypeFor[ApprovalRuleScope]():         {string(ApprovalRuleScopeSession), string(ApprovalRuleScopeProject), string(ApprovalRuleScopeGlobal)},
	reflect.TypeFor[ArtifactOutcomeType]():       {string(ArtifactOutcomeCompleted), string(ArtifactOutcomeError), string(ArtifactOutcomeMaxSteps), string(ArtifactOutcomeMaxBudget), string(ArtifactOutcomeCanceled)},
	reflect.TypeFor[ArtifactProblemType]():       {string(ArtifactProblemInternalError), string(ArtifactProblemRunLost), string(ArtifactProblemAgentStuck), string(ArtifactProblemRateLimited), string(ArtifactProblemInvalidAPIKey), string(ArtifactProblemTimeout), string(ArtifactProblemProviderUnavailable), string(ArtifactProblemProviderRejected), string(ArtifactProblemDeniedByUser), string(ArtifactProblemToolFailed)},
	reflect.TypeFor[CodebaseState]():             {string(CodebaseStateNone), string(CodebaseStateIndexing), string(CodebaseStateReady), string(CodebaseStateError)},
	reflect.TypeFor[ContentBlockType]():          {string(ContentBlockText), string(ContentBlockImage)},
	reflect.TypeFor[DiffFormat]():                {string(DiffFormatRows), string(DiffFormatRaw)},
	reflect.TypeFor[DiffMode]():                  {string(DiffModeWorktree), string(DiffModeBase)},
	reflect.TypeFor[DiffRowType]():               {string(DiffRowHunk), string(DiffRowContext), string(DiffRowAdded), string(DiffRowDeleted)},
	reflect.TypeFor[ErrorChannel]():              {string(ErrorChannelRPC), string(ErrorChannelRun), string(ErrorChannelTool)},
	reflect.TypeFor[ExportFormat]():              {string(ExportFormatMarkdown), string(ExportFormatJSON)},
	reflect.TypeFor[FeedbackRating]():            {string(FeedbackPositive), string(FeedbackNegative)},
	reflect.TypeFor[FileEntryType]():             {string(FileEntryFile), string(FileEntryDir), string(FileEntrySymlink)},
	reflect.TypeFor[FileStatus]():                {string(FileStatusAdded), string(FileStatusModified), string(FileStatusDeleted), string(FileStatusRenamed), string(FileStatusUntracked)},
	reflect.TypeFor[GoalStatus]():                {string(GoalActive), string(GoalPaused), string(GoalBlocked)},
	reflect.TypeFor[HookEvent]():                 {string(HookEventPreToolUse), string(HookEventPostToolUse), string(HookEventUserPromptSubmit), string(HookEventSessionStart), string(HookEventSubagentStart), string(HookEventSubagentStop), string(HookEventPreCompact), string(HookEventStop), string(HookEventNotification)},
	reflect.TypeFor[HookScope]():                 {string(HookScopeGlobal), string(HookScopeProject)},
	reflect.TypeFor[InterruptResponseType]():     {string(InterruptResponseApproval), string(InterruptResponseAnswer), string(InterruptResponseToolResult)},
	reflect.TypeFor[InterruptType]():             {string(InterruptApproval), string(InterruptQuestion), string(InterruptToolResult)},
	reflect.TypeFor[ItemDeltaType]():             {string(DeltaContent), string(DeltaReasoning), string(DeltaToolArguments), string(DeltaToolOutput), string(DeltaPlan)},
	reflect.TypeFor[ItemOrder]():                 {string(ItemOrderAsc), string(ItemOrderDesc)},
	reflect.TypeFor[ItemScopeType]():             {string(ItemScopeSession), string(ItemScopeRun)},
	reflect.TypeFor[ItemStatus]():                {string(ItemStatusRunning), string(ItemStatusCompleted), string(ItemStatusIncomplete)},
	reflect.TypeFor[ItemType]():                  {string(ItemTypeUserMessage), string(ItemTypeAgentMessage), string(ItemTypeReasoning), string(ItemTypePlan), string(ItemTypeQuestion), string(ItemTypeToolCall), string(ItemTypeCompaction)},
	reflect.TypeFor[McpAuthStatus]():             {string(McpAuthNone), string(McpAuthBearerToken), string(McpAuthOAuth), string(McpAuthNotLoggedIn)},
	reflect.TypeFor[McpStatus]():                 {string(McpConnecting), string(McpConnected), string(McpDisconnected), string(McpFailed), string(McpNeedsAuth)},
	reflect.TypeFor[McpTransport]():              {string(McpTransportStdio), string(McpTransportStreamableHTTP)},
	reflect.TypeFor[MemoryScope]():               {string(MemoryScopeCwd), string(MemoryScopeProjectRoot), string(MemoryScopeHome)},
	reflect.TypeFor[Modality]():                  {string(ModalityText), string(ModalityImage), string(ModalityAudio), string(ModalityVideo), string(ModalityPDF)},
	reflect.TypeFor[PlanStepStatus]():            {string(PlanStepPending), string(PlanStepRunning), string(PlanStepCompleted), string(PlanStepFailed)},
	reflect.TypeFor[ProviderKeySource]():         {string(ProviderKeySourceStored), string(ProviderKeySourceEnv)},
	reflect.TypeFor[QuestionFieldType]():         {string(QuestionFieldText), string(QuestionFieldChoice)},
	reflect.TypeFor[RecipeScope]():               {string(RecipeScopeProject), string(RecipeScopeGlobal)},
	reflect.TypeFor[RememberScopeKind]():         {string(RememberSession), string(RememberProject), string(RememberGlobal)},
	reflect.TypeFor[RestoreType]():               {string(RestoreHistory), string(RestoreFiles), string(RestoreBoth)},
	reflect.TypeFor[RunOutcomeType]():            {string(OutcomeCompleted), string(OutcomeError), string(OutcomeMaxSteps), string(OutcomeMaxBudget), string(OutcomeCanceled)},
	reflect.TypeFor[RunStatus]():                 {string(RunStatusRunning), string(RunStatusWaiting), string(RunStatusFinished)},
	reflect.TypeFor[SegmentOutcomeType]():        {string(SegmentInterrupt), string(SegmentSuspended), string(SegmentCompleted), string(SegmentError), string(SegmentMaxSteps), string(SegmentMaxBudget), string(SegmentCanceled)},
	reflect.TypeFor[SafetyClass]():               {string(SafetyClassSafe), string(SafetyClassWrite), string(SafetyClassExec), string(SafetyClassNetwork)},
	reflect.TypeFor[SessionStatus]():             {string(SessionStatusRunning), string(SessionStatusWaiting), string(SessionStatusIdle)},
	reflect.TypeFor[SkillLifecycle]():            {string(SkillLifecycleActive), string(SkillLifecycleArchived)},
	reflect.TypeFor[SkillSource]():               {string(SkillSourceProject), string(SkillSourceGlobal)},
	reflect.TypeFor[Stability]():                 {string(StabilityStable), string(StabilityExperimental)},
	reflect.TypeFor[StreamEventType]():           {string(StreamSegmentStarted), string(StreamSegmentProgress), string(StreamSegmentFinished), string(StreamItemStarted), string(StreamItemDelta), string(StreamItemCompleted), string(StreamStateSnapshot), string(StreamCustom)},
	reflect.TypeFor[TodoStatus]():                {string(TodoStatusPending), string(TodoStatusInProgress), string(TodoStatusCompleted)},
	reflect.TypeFor[WorkspaceEventType]():        {string(WorkspaceEventFilesChanged), string(WorkspaceEventSkillsChanged), string(WorkspaceEventMCPServerChanged), string(WorkspaceEventSchedulesFired), string(WorkspaceEventResync)},
}

// WireEnum reports the closed value set of a wire enum type, and false when t is
// not one. Callers are build-time artifact generators and the wire validator;
// both read the set, neither owns it.
func WireEnum(t reflect.Type) ([]string, bool) {
	values, ok := wireEnums[t]
	return values, ok
}
