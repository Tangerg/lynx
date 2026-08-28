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
  useCurrentRootRun,
  useAgentProblem,
  useAgentPlan,
  useRootNarrativeMessages,
  useRunTree,
  useTranscriptRows,
  useAgentSessionTimeline,
  useAgentSharedMaterial,
  useAgentToolCalls,
} from "./agentViewSelectors";
import { AgentViewRefreshOwner, useAgentStore } from "./agentStore";

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
 * Enter a Session identity. Two halves of one move: the tab set remembers it is
 * open, and the location says it is where the user is. Promoted material belongs
 * to the predecessor Session and retires at this identity boundary; the dock is
 * deliberately omitted because its per-Session scope owner restores that memory.
 */
function goToSession(id: string, options?: { replace?: boolean }): void {
  const store = useAgentSessionStore.getState();
  if (id !== "") store.holdOpen(id);
  // Recorded here rather than by a subscriber on the location: ports are
  // installed while plugins load, which is before the router (and so the
  // Navigator) exists. Memory is a consequence of the move, so the mover keeps
  // it — and there is one mover.
  store.rememberSession(id);
  navigator().go({ session: id, view: null }, options);
}

export function installAgentStatePorts(): () => void {
  // Claim before publishing successor ports. From this point an old port retained by
  // an in-flight snapshot can no longer commit into the shared material store.
  const refreshOwner = AgentViewRefreshOwner.install();
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
      const currentSessionId = activeSessionId();
      const next = closeOpenSession(
        { activeSessionId: currentSessionId, openSessionIds: store.openSessionIds },
        id,
      );
      store.release(id);
      if (next.activeSessionId === currentSessionId) {
        store.rememberSession(next.activeSessionId);
        return;
      }
      goToSession(next.activeSessionId);
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
          provisionalSessionIds: store.freshDraftSessionIds,
        },
        liveIds,
      );
      if (!next) return;
      store.retainOnly(next.openSessionIds);
      // Reconciliation may correct a deleted deep-link/last-session seed or
      // establish a direct live deep-link as held-open. Keep cold-start memory
      // aligned with that accepted location; otherwise the next launch replays
      // the identity we just proved stale.
      if (next.activeSessionId !== activeSessionId()) {
        goToSession(next.activeSessionId, { replace: true });
      } else {
        store.rememberSession(next.activeSessionId);
      }
    },
    restoreLastSession: () => {
      if (activeSessionId() !== "") return;
      const { lastSessionId } = useAgentSessionStore.getState();
      if (lastSessionId === "") return;
      goToSession(lastSessionId);
    },
    markDraftSession: (id) => useAgentSessionStore.getState().markDraft(id),
  });

  const disposeViewState = configureAgentSessionViewPort({
    useCurrentRootRun,
    useToolCalls: useAgentToolCalls,
    useSessionTimeline: useAgentSessionTimeline,
    useRootNarrativeMessages,
    useTranscriptRows,
    useRunTree,
    useProblem: useAgentProblem,
    usePlan: useAgentPlan,
    useSharedMaterial: useAgentSharedMaterial,
    useAction: useAgentAction,
    getCurrentView: getCurrentSessionView,
    getSessions: () => useAgentStore.getState().sessions,
    getSession: (sessionId) => useAgentStore.getState().sessions[sessionId],
    sendToSession: (sessionId, input, options) => {
      const send = useAgentStore.getState().sessions[sessionId]?.send;
      if (!send) return false;
      return send(input, options);
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
      refreshOwner.begin(sessionId, invalidateQueuedRunEvents),
    commitViewRefresh: (sessionId, token, view) => refreshOwner.commit(sessionId, token, view),
    retireProjectionGeneration: (sessionIds) => refreshOwner.retireProjectionGeneration(sessionIds),
    replaceServerScope: (sessionIds) => refreshOwner.replaceServerScope(sessionIds),
    clearProblem: (sessionId) => useAgentStore.getState().clearProblem(sessionId),
    resolveInterrupt: (sessionId, itemId, settled, resolvedAt) =>
      useAgentStore.getState().resolveInterrupt(sessionId, itemId, settled, resolvedAt),
    subscribeSessions: (onChange) => useAgentStore.subscribe((state) => onChange(state.sessions)),
  });
  return () => {
    refreshOwner.dispose();
    disposeViewState();
    disposeSessionState();
  };
}
