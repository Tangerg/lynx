import { agentInputToContentBlocks } from "./wireInput";
import { localUserMessage } from "./optimisticUserMessage";
import { useAgentSessionStore } from "./agentSessionStore";
import {
  configureAgentSessionStatePort,
  type AgentSessionLifecycleSnapshot,
  type AgentSessionSelectionSnapshot,
} from "../application/ports/sessionState";
import { configureAgentSessionViewPort } from "../application/ports/sessionView";
import {
  getCurrentSessionView,
  useAgentAction,
  useCurrentRootAttention,
  useDelegatedRunNarratives,
  useAgentProblem,
  useRootNarrativeMessages,
  useRunTree,
  useAgentSessionTimeline,
  useAgentSharedState,
  useAgentToolCalls,
  useCurrentRootContextTokens,
  useCurrentRootOutcome,
  useCurrentRootMetrics,
  useCurrentRootRunId,
  useCurrentRootSegmentId,
  useCurrentRootUsage,
} from "./agentViewSelectors";
import { useAgentStore } from "./agentStore";

function getLifecycleSnapshot(): AgentSessionLifecycleSnapshot {
  const state = useAgentSessionStore.getState();
  return { activeSessionId: state.activeSessionId, openSessionIds: state.openSessionIds };
}

function getSelectionSnapshot(): AgentSessionSelectionSnapshot {
  const state = useAgentSessionStore.getState();
  return { activeSessionId: state.activeSessionId, selectionEpoch: state.selectionEpoch };
}

export function installAgentStatePorts(): () => void {
  const disposeSessionState = configureAgentSessionStatePort({
    useActiveSessionId: () => useAgentSessionStore((state) => state.activeSessionId),
    getActiveSessionId: () => useAgentSessionStore.getState().activeSessionId,
    getLifecycleSnapshot,
    subscribeActiveSessionId: (onChange) => {
      let lastSessionId = useAgentSessionStore.getState().activeSessionId;
      return useAgentSessionStore.subscribe((state) => {
        if (state.activeSessionId === lastSessionId) return;
        lastSessionId = state.activeSessionId;
        onChange(lastSessionId);
      });
    },
    subscribeLifecycle: (onChange) => {
      let lastSnapshot = getLifecycleSnapshot();
      return useAgentSessionStore.subscribe((state) => {
        if (
          state.activeSessionId === lastSnapshot.activeSessionId &&
          state.openSessionIds === lastSnapshot.openSessionIds
        ) {
          return;
        }
        lastSnapshot = {
          activeSessionId: state.activeSessionId,
          openSessionIds: state.openSessionIds,
        };
        onChange(lastSnapshot);
      });
    },
    subscribeSelection: (onChange) => {
      let lastSnapshot = getSelectionSnapshot();
      return useAgentSessionStore.subscribe((state) => {
        if (
          state.activeSessionId === lastSnapshot.activeSessionId &&
          state.selectionEpoch === lastSnapshot.selectionEpoch
        ) {
          return;
        }
        const previous = lastSnapshot;
        lastSnapshot = {
          activeSessionId: state.activeSessionId,
          selectionEpoch: state.selectionEpoch,
        };
        onChange(lastSnapshot, previous);
      });
    },
    selectSession: (id) => useAgentSessionStore.getState().selectSession(id),
    closeSession: (id) => useAgentSessionStore.getState().closeSession(id),
    useDraftSessionIds: () => useAgentSessionStore((state) => state.draftSessionIds),
    isDraftSession: (id) => useAgentSessionStore.getState().draftSessionIds.has(id),
    useSelectSession: () => useAgentSessionStore((state) => state.selectSession),
    reconcileSessions: (liveIds) => useAgentSessionStore.getState().reconcileSessions(liveIds),
    markDraftSession: (id) => useAgentSessionStore.getState().markDraft(id),
    graduateDraftSession: (id) => useAgentSessionStore.getState().graduateDraft(id),
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
    useDelegatedRunNarratives,
    useRunTree,
    useProblem: useAgentProblem,
    useSharedState: useAgentSharedState,
    useCurrentRootUsage,
    useCurrentRootContextTokens,
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
