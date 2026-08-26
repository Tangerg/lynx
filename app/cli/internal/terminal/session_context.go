package terminal

import "github.com/Tangerg/lynx/app/cli/internal/agent"

type sessionContextEpoch uint64

func (s *sessionContextEpoch) advance() {
	*s = *s + 1
}

func (a *app) canPreserveInteractionProjection(next *agent.Conversation) bool {
	return a.interactionReview != nil && a.conversation.Phase() == agent.ConversationWaiting &&
		next != nil && next.Phase() == agent.ConversationWaiting && a.conversation.RunID() == next.RunID() &&
		sameInteractions(a.conversation.Interactions(), next.Interactions())
}

func (a *app) prepareSessionProjectionReplacement(next agent.Session, conversation *agent.Conversation) {
	if next.ID != a.session.ID || next.Workspace != a.session.Workspace {
		a.retireSessionContext()
		return
	}
	if a.reader.ObservingSource() {
		a.dismissReader()
	}
	if !a.canPreserveInteractionProjection(conversation) {
		a.dismissInteractionProjection()
	}
}

func (a *app) retireSessionContext() {
	a.sessionContext.advance()
	a.dismissInteractionProjection()
	a.dismissConfirmation()
	a.dismissReader()
	a.dismissContextEditor()
	a.searchDialog.Dismiss()
	a.commandDialog.Dismiss()
	a.timelineDialog.Dismiss()
	a.workspaceDialog.Dismiss()
	a.modelDialog.Dismiss()
	a.sessionDialog.Dismiss()
	a.queueDialog.Dismiss()
	if a.sessionRenameDialog != nil {
		a.sessionRenameDialog.Dismiss()
		a.sessionRenameDialog = nil
	}
	if a.sessionDeleteDialog != nil {
		a.sessionDeleteDialog.Dismiss()
		a.sessionDeleteDialog = nil
	}
	if a.scheduleDialog != nil {
		a.scheduleDialog.Dismiss()
		a.scheduleDialog = nil
	}
}
