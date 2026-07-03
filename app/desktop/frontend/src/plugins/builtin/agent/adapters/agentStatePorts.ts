import type { RunEvent } from "@/rpc";
import { agentInputToContentBlocks } from "./wireInput";
import { useAgentSessionStore } from "./agentSessionStore";
import {
  configureAgentSessionStatePort,
  type AgentSessionLifecycleSnapshot,
  type AgentSessionSelectionSnapshot,
} from "../application/ports/sessionState";
import { configureAgentViewStatePort } from "../application/ports/viewState";
import {
  getCurrentSessionView,
  useAgentAction,
  useAgentError,
  useAgentMessages,
  useAgentPlan,
  useAgentRunContextTokens,
  useAgentRunId,
  useAgentRunning,
  useAgentRunUsage,
  useAgentSharedState,
  useAgentTimeline,
  useAgentToolCalls,
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

export function installAgentStatePorts(): void {
  configureAgentSessionStatePort({
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
    useSelectSession: () => useAgentSessionStore((state) => state.selectSession),
    reconcileSessions: (liveIds) => useAgentSessionStore.getState().reconcileSessions(liveIds),
    markDraftSession: (id) => useAgentSessionStore.getState().markDraft(id),
    graduateDraftSession: (id) => useAgentSessionStore.getState().graduateDraft(id),
    setPendingMessage: (id, message) =>
      useAgentSessionStore.getState().setPendingMessage(id, message),
    takePendingMessage: (id) => useAgentSessionStore.getState().takePendingMessage(id),
  });

  configureAgentViewStatePort({
    useRunning: useAgentRunning,
    useRunId: useAgentRunId,
    usePlan: useAgentPlan,
    useToolCalls: useAgentToolCalls,
    useTimeline: useAgentTimeline,
    useMessages: useAgentMessages,
    useError: useAgentError,
    useSharedState: useAgentSharedState,
    useUsage: useAgentRunUsage,
    useContextTokens: useAgentRunContextTokens,
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
    appendLocalUserMessage: (sessionId, messageId, input) => {
      useAgentStore.getState().applyEvents(sessionId, [
        {
          event: {
            type: "item.completed",
            item: {
              id: messageId,
              runId: "",
              status: "completed",
              createdAt: new Date().toISOString(),
              type: "userMessage",
              content: agentInputToContentBlocks(input),
            },
          } as RunEvent["event"],
        },
      ]);
    },
    resetView: (sessionId) => useAgentStore.getState().resetView(sessionId),
    applyCompletedItems: (sessionId, items) =>
      useAgentStore.getState().applyEvents(
        sessionId,
        items.map((item) => ({ event: { type: "item.completed" as const, item } })),
      ),
    clearError: (sessionId) => useAgentStore.getState().clearError(sessionId),
    resolveInterrupt: (sessionId, itemId, settled) =>
      useAgentStore.getState().resolveInterrupt(sessionId, itemId, settled),
    subscribeSessions: (onChange) => useAgentStore.subscribe((state) => onChange(state.sessions)),
  });
}
