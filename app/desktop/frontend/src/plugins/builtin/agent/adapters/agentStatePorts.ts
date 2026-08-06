import { agentInputToContentBlocks } from "./wireInput";
import { localUserMessage } from "./optimisticUserMessage";
import { useAgentSessionStore } from "./agentSessionStore";
import { navigator } from "@/lib/navigation";
import {
  closeOpenSession,
  reconcileOpenSessions,
} from "../application/session/sessionSelectionModel";
import {
  configureAgentSessionStatePort,
  type AgentSessionLifecycleSnapshot,
} from "../application/ports/sessionState";
import { configureAgentSessionViewPort } from "../application/ports/sessionView";
import {
  getCurrentSessionView,
  useAgentAction,
  useCurrentRootAttention,
  useAgentProblem,
  useRootNarrativeMessages,
  useRunTree,
  useTranscriptRows,
  useAgentSessionTimeline,
  useAgentSharedState,
  useAgentToolCalls,
  useCurrentRootOutcome,
  useCurrentRootMetrics,
  useCurrentRootRunId,
  useCurrentRootSegmentId,
} from "./agentViewSelectors";
import { useAgentStore } from "./agentStore";

function activeSessionId(): string {
  return navigator().get().session;
}

function getLifecycleSnapshot(): AgentSessionLifecycleSnapshot {
  return {
    activeSessionId: activeSessionId(),
    openSessionIds: useAgentSessionStore.getState().openSessionIds,
  };
}

/**
 * Go to a session. Two halves of one move: the tab set remembers it is open, and
 * the location says it is where the user is — leaving any promoted view, so
 * picking a session lands you in its conversation.
 */
function goToSession(id: string): void {
  const store = useAgentSessionStore.getState();
  store.holdOpen(id);
  // Recorded here rather than by a subscriber on the location: ports are
  // installed while plugins load, which is before the router (and so the
  // Navigator) exists. Memory is a consequence of the move, so the mover keeps
  // it — and there is one mover.
  store.rememberSession(id);
  navigator().go({ session: id, view: null });
}

export function installAgentStatePorts(): () => void {
  const disposeSessionState = configureAgentSessionStatePort({
    useActiveSessionId: () => navigator().use((location) => location.session),
    getActiveSessionId: activeSessionId,
    getLifecycleSnapshot,
    subscribeActiveSessionId: (onChange) =>
      navigator().subscribe((location, previous) => {
        if (location.session !== previous.session) onChange(location.session);
      }),
    // Fires for either half: the location moved to another session, or the open
    // set changed under the one we are on.
    subscribeLifecycle: (onChange) => {
      let last = getLifecycleSnapshot();
      const emit = () => {
        const next = getLifecycleSnapshot();
        if (
          next.activeSessionId === last.activeSessionId &&
          next.openSessionIds === last.openSessionIds
        ) {
          return;
        }
        last = next;
        onChange(next);
      };
      const unsubscribeLocation = navigator().subscribe(emit);
      const unsubscribeStore = useAgentSessionStore.subscribe(emit);
      return () => {
        unsubscribeLocation();
        unsubscribeStore();
      };
    },
    selectSession: goToSession,
    closeSession: (id) => {
      const store = useAgentSessionStore.getState();
      const next = closeOpenSession(
        { activeSessionId: activeSessionId(), openSessionIds: store.openSessionIds },
        id,
      );
      store.release(id);
      navigator().go({ session: next.activeSessionId });
    },
    useDraftSessionIds: () => useAgentSessionStore((state) => state.draftSessionIds),
    isDraftSession: (id) => useAgentSessionStore.getState().draftSessionIds.has(id),
    // Boot: drop open + active refs to sessions the runtime no longer has. The
    // location is corrected with `replace` — a session that turned out not to
    // exist was never a place the user went, so there is nothing to go back to.
    reconcileSessions: (liveIds) => {
      const store = useAgentSessionStore.getState();
      const next = reconcileOpenSessions(
        {
          activeSessionId: activeSessionId(),
          openSessionIds: store.openSessionIds,
          draftSessionIds: store.draftSessionIds,
        },
        liveIds,
      );
      if (!next) return;
      store.retainOnly(next.openSessionIds);
      if (next.activeSessionId !== activeSessionId()) {
        navigator().go({ session: next.activeSessionId }, { replace: true });
      }
    },
    restoreLastSession: () => {
      if (activeSessionId() !== "") return;
      const { lastSessionId } = useAgentSessionStore.getState();
      if (lastSessionId === "") return;
      goToSession(lastSessionId);
    },
    markDraftSession: (id) => useAgentSessionStore.getState().markDraft(id),
    setPendingMessage: (id, message) =>
      useAgentSessionStore.getState().setPendingMessage(id, message),
    takePendingMessage: (id) => useAgentSessionStore.getState().takePendingMessage(id),
  });

  const disposeViewState = configureAgentSessionViewPort({
    useCurrentRootAttention,
    useCurrentRootOutcome,
    useCurrentRootMetrics,
    useCurrentRootRunId,
    useCurrentRootSegmentId,
    useToolCalls: useAgentToolCalls,
    useSessionTimeline: useAgentSessionTimeline,
    useRootNarrativeMessages,
    useTranscriptRows,
    useRunTree,
    useProblem: useAgentProblem,
    useSharedState: useAgentSharedState,
    useAction: useAgentAction,
    getCurrentView: getCurrentSessionView,
    getSessions: () => useAgentStore.getState().sessions,
    getSession: (sessionId) => useAgentStore.getState().sessions[sessionId],
    sendToSession: (sessionId, input, options) => {
      const send = useAgentStore.getState().sessions[sessionId]?.send;
      if (!send) return false;
      send(input, options);
      return true;
    },
    dropMessage: (sessionId, messageId) =>
      useAgentStore.getState().dropMessage(sessionId, messageId),
    appendLocalUserMessage: (sessionId, messageId, input) =>
      useAgentStore
        .getState()
        .appendLocalMessage(
          sessionId,
          localUserMessage(messageId, agentInputToContentBlocks(input)),
        ),
    beginViewRefresh: (sessionId, invalidateQueuedRunEvents) =>
      useAgentStore.getState().beginViewRefresh(sessionId, invalidateQueuedRunEvents),
    commitViewRefresh: (sessionId, token, view) =>
      useAgentStore.getState().commitViewRefresh(sessionId, token, view),
    clearProblem: (sessionId) => useAgentStore.getState().clearProblem(sessionId),
    resolveInterrupt: (sessionId, itemId, settled, resolvedAt) =>
      useAgentStore.getState().resolveInterrupt(sessionId, itemId, settled, resolvedAt),
    subscribeSessions: (onChange) => useAgentStore.subscribe((state) => onChange(state.sessions)),
  });
  return () => {
    disposeViewState();
    disposeSessionState();
  };
}
