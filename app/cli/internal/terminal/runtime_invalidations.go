package terminal

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
)

var runtimeRecoveryBackoff = retry.Backoff{Base: 100 * time.Millisecond, Maximum: 5 * time.Second}

func (a *app) applyRuntimeInvalidation(event changefeed.Event) {
	if event.Type == changefeed.Resync {
		a.applyRuntimeResync(event.Topics)
		return
	}
	a.refreshGoalReader(goalInvalidationAffectsSession(event, a.session.ID))
	a.refreshSkillReader(changefeed.Topic(event.Type) == changefeed.SkillsChanged)
	a.refreshMCPReader(changefeed.Topic(event.Type) == changefeed.MCPChanged)
	a.refreshScheduleReader(changefeed.Topic(event.Type) == changefeed.SchedulesChanged)
	a.refreshKnowledgeReader(changefeed.Topic(event.Type) == changefeed.KnowledgeChanged)
	a.refreshHooksReader(changefeed.Topic(event.Type) == changefeed.HooksChanged)
	a.refreshModelReader(changefeed.Topic(event.Type) == changefeed.ModelsChanged)
	a.refreshApprovalReader(changefeed.Topic(event.Type) == changefeed.ApprovalsChanged)
	a.refreshAgentMemoryReader(changefeed.Topic(event.Type) == changefeed.AgentMemoryChanged)
	a.refreshCodebaseReader(changefeed.Topic(event.Type) == changefeed.CodebaseChanged)
	a.applySessionInvalidation(
		invalidatesSessionCatalog(event),
		invalidationAffectsSession(event, a.session.ID, a.conversation.RunID()),
	)
}

func (a *app) applyRuntimeResync(topics []changefeed.Topic) {
	a.refreshGoalReader(containsTopic(topics, changefeed.GoalsChanged))
	a.refreshSkillReader(containsTopic(topics, changefeed.SkillsChanged))
	a.refreshMCPReader(containsTopic(topics, changefeed.MCPChanged))
	a.refreshScheduleReader(containsTopic(topics, changefeed.SchedulesChanged))
	a.refreshKnowledgeReader(containsTopic(topics, changefeed.KnowledgeChanged))
	a.refreshHooksReader(containsTopic(topics, changefeed.HooksChanged))
	a.refreshModelReader(containsTopic(topics, changefeed.ModelsChanged))
	a.refreshApprovalReader(containsTopic(topics, changefeed.ApprovalsChanged))
	a.refreshAgentMemoryReader(containsTopic(topics, changefeed.AgentMemoryChanged))
	a.refreshCodebaseReader(containsTopic(topics, changefeed.CodebaseChanged))
	a.applySessionInvalidation(
		invalidatesSessionCatalog(changefeed.Event{Type: changefeed.Resync, Topics: topics}),
		resyncAffectsSession(topics),
	)
}

func (a *app) refreshKnowledgeReader(affected bool) {
	if !affected || a.knowledge == nil || a.runtimeReader != runtimeReaderKnowledge || !a.readerDialog.Open() {
		return
	}
	if a.runtimeSelection.knowledgeEntry {
		a.refreshRuntimeReader(a.knowledgeDocumentReaderQuery(a.runtimeSelection.knowledgeTarget))
		return
	}
	a.refreshRuntimeReader(a.knowledgeEntriesReaderQuery())
}

func (a *app) refreshHooksReader(affected bool) {
	if affected && a.hooks != nil && a.runtimeReader == runtimeReaderHooks && a.readerDialog.Open() {
		a.refreshRuntimeReader(a.hooksReaderQuery())
	}
}

func (a *app) refreshScheduleReader(affected bool) {
	if affected && a.schedules != nil && a.runtimeReader == runtimeReaderSchedules && a.readerDialog.Open() {
		a.refreshRuntimeReader(a.schedulesReaderQuery())
	}
}

func (a *app) refreshMCPReader(affected bool) {
	if !affected || a.mcp == nil || !a.readerDialog.Open() {
		return
	}
	switch a.runtimeReader {
	case runtimeReaderMCPServers:
		a.refreshRuntimeReader(a.mcpServersReaderQuery())
	case runtimeReaderMCPTools:
		a.refreshRuntimeReader(a.mcpToolsReaderQuery(a.mcpToolServer))
	}
}

func (a *app) refreshSkillReader(affected bool) {
	if !affected || a.skills == nil || !a.readerDialog.Open() {
		return
	}
	switch a.runtimeReader {
	case runtimeReaderDiscoveredSkills:
		a.refreshRuntimeReader(a.discoveredSkillsReaderQuery())
	case runtimeReaderManagedSkills:
		a.refreshRuntimeReader(a.managedSkillsReaderQuery())
	case runtimeReaderSkillProposals:
		a.refreshRuntimeReader(a.skillProposalsReaderQuery())
	}
}

func (a *app) refreshGoalReader(affected bool) {
	if affected && a.goals != nil && a.runtimeReader == runtimeReaderGoal && a.readerDialog.Open() {
		a.refreshRuntimeReader(a.goalReaderQuery())
	}
}

func (a *app) refreshModelReader(affected bool) {
	if !affected {
		return
	}
	if a.modelDialog.Open() {
		a.loadModelPicker(false)
	}
	if !a.readerDialog.Open() {
		return
	}
	switch a.runtimeReader {
	case runtimeReaderModels:
		a.refreshRuntimeReader(a.modelsReaderQuery())
	case runtimeReaderModelRoles:
		if a.modelConfig != nil {
			a.refreshRuntimeReader(a.modelRolesReaderQuery())
		}
	case runtimeReaderProviders:
		if a.modelConfig != nil {
			a.refreshRuntimeReader(a.providersReaderQuery())
		}
	}
}

func (a *app) refreshApprovalReader(affected bool) {
	if affected && a.runtimeReader == runtimeReaderApprovalRules && a.readerDialog.Open() {
		a.refreshRuntimeReader(a.approvalRulesReaderQuery())
	}
}

func (a *app) refreshAgentMemoryReader(affected bool) {
	if affected && a.agentMemory != nil && a.runtimeReader == runtimeReaderAgentMemory && a.readerDialog.Open() {
		a.refreshRuntimeReader(a.agentMemoryReaderQuery(a.runtimeSelection.agentMemoryTarget))
	}
}

func (a *app) refreshCodebaseReader(affected bool) {
	if !affected || a.codebase == nil || !a.readerDialog.Open() {
		return
	}
	switch a.runtimeReader {
	case runtimeReaderCodebaseStatus:
		a.refreshRuntimeReader(a.codebaseStatusReaderQuery(a.session.Workspace.Path))
	case runtimeReaderCodebaseSearch:
		a.refreshRuntimeReader(a.codebaseSearchReaderQuery(a.runtimeSelection.codebaseQuery))
	}
}

func (a *app) refreshRuntimeReader(query runtimeReaderQuery) {
	read := query.read
	query.read = func(ctx context.Context) (readerDocument, error) {
		failures := 0
		for {
			document, err := read(ctx)
			if err == nil || !reconnect.Retryable(err) {
				return document, err
			}
			failures++
			if err := retry.Wait(ctx, runtimeRecoveryBackoff.Delay(failures)); err != nil {
				return readerDocument{}, err
			}
		}
	}
	a.executeRuntimeReaderQuery(query)
}

func goalInvalidationAffectsSession(event changefeed.Event, sessionID string) bool {
	if event.Type == changefeed.Resync {
		return containsTopic(event.Topics, changefeed.GoalsChanged)
	}
	return changefeed.Topic(event.Type) == changefeed.GoalsChanged &&
		(len(event.SessionIDs) == 0 || containsString(event.SessionIDs, sessionID))
}

func (a *app) applySessionInvalidation(catalogChanged, currentSessionChanged bool) {
	if catalogChanged && a.sessionDialog.Open() {
		a.sessionCenter.Reset()
		a.loadSessionPage("", false)
	}
	if !currentSessionChanged {
		return
	}
	a.sessionInvalidated = true
	if a.conversation.Phase() == agent.ConversationRunning || a.following || a.pendingCancel != nil ||
		a.operations.Active(sessionChangeOperation) {
		return
	}
	a.refreshInvalidatedSession(false)
}

func invalidatesSessionCatalog(event changefeed.Event) bool {
	if event.Type == changefeed.Resync {
		return containsTopic(event.Topics, changefeed.SessionsChanged) ||
			containsTopic(event.Topics, changefeed.RunsChanged)
	}
	return event.Type == changefeed.EventType(changefeed.SessionsChanged) ||
		event.Type == changefeed.EventType(changefeed.RunsChanged)
}

func resyncAffectsSession(topics []changefeed.Topic) bool {
	return slices.ContainsFunc(topics, func(topic changefeed.Topic) bool {
		return topic == changefeed.SessionsChanged || topic == changefeed.RunsChanged ||
			topic == changefeed.StateChanged || topic == changefeed.InterruptsChanged
	})
}

func invalidationAffectsSession(event changefeed.Event, sessionID, runID string) bool {
	if event.Type == changefeed.Resync {
		return resyncAffectsSession(event.Topics)
	}
	switch changefeed.Topic(event.Type) {
	case changefeed.SessionsChanged:
		return len(event.SessionIDs) == 0 || containsString(event.SessionIDs, sessionID)
	case changefeed.StateChanged:
		return event.StateKey == changefeed.StatePlan &&
			(len(event.SessionIDs) == 0 || containsString(event.SessionIDs, sessionID))
	case changefeed.RunsChanged, changefeed.InterruptsChanged:
		if len(event.SessionIDs) != 0 {
			return containsString(event.SessionIDs, sessionID)
		}
		return len(event.RunIDs) == 0 || containsString(event.RunIDs, runID)
	default:
		return false
	}
}

func (a *app) refreshInvalidatedSession(settleAfter bool) {
	sessionID := a.session.ID
	a.sessionInvalidated = false
	started := runOperation(a, sessionInvalidationOperation, false,
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			return a.readInvalidatedSession(ctx, sessionID)
		},
		func(snapshot agent.SessionSnapshot, err error) {
			if a.session.ID != sessionID {
				return
			}
			if a.sessionInvalidated {
				a.refreshInvalidatedSession(settleAfter)
				return
			}
			if err != nil {
				a.sessionInvalidated = true
				if errors.Is(err, agent.ErrSessionNotFound) && a.conversation.Phase() == agent.ConversationIdle && !a.following {
					a.message("the active session was deleted; creating a replacement")
					a.replaceDeletedSessionInWorkspace(a.session.Workspace.Path)
					return
				}
				a.message("refresh session after runtime change failed: " + err.Error())
				return
			}
			if a.conversation.Phase() == agent.ConversationRunning || a.following {
				a.sessionInvalidated = true
				return
			}
			if a.conversation.MatchesSnapshot(snapshot) && a.session.Workspace == snapshot.Session.Workspace {
				a.installSessionMetadata(snapshot.Session)
			} else {
				if err := a.installSnapshot(snapshot); err != nil {
					a.message("refresh session after runtime change failed: " + err.Error())
					return
				}
			}
			if settleAfter && a.conversation.Phase() == agent.ConversationIdle {
				a.finishFollowing()
				return
			}
			a.message("session refreshed after runtime change")
		},
	)
	if !started {
		a.sessionInvalidated = true
	}
}

func (a *app) readInvalidatedSession(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	failures := 0
	for {
		snapshot, err := a.runtime.GetSession(ctx, sessionID)
		if err == nil || !reconnect.Retryable(err) {
			return snapshot, err
		}
		failures++
		if err := retry.Wait(ctx, runtimeRecoveryBackoff.Delay(failures)); err != nil {
			return agent.SessionSnapshot{}, err
		}
	}
}

func (a *app) installSessionMetadata(session agent.Session) {
	a.setActiveSession(session)
	a.sessionCenter.Upsert(session)
}

// dismissInteractionProjection drops only the obsolete terminal-side answer
// draft. It never answers or cancels the runtime interaction.
func (a *app) dismissInteractionProjection() {
	a.clearApprovalProjection()
	a.questionnaire = nil
	a.interactionReview = nil
	if a.approvalDialog != nil {
		a.approvalDialog.Dismiss()
	}
	if a.questionDialog != nil {
		a.questionDialog.Dismiss()
		a.questionDialog = nil
	}
	if a.reviewDialog != nil {
		a.reviewDialog.Dismiss()
		a.reviewDialog = nil
	}
}
