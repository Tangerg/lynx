import type { CancelRunResponse, RunEvent, RunRef } from "@/rpc";
import type {
  AgentViewRefreshToken,
  CancelRunAction,
  ResumeRunAction,
  SendAgentInputAction,
  StopCurrentRootRunAction,
  SynchronizeSessionAction,
} from "@/plugins/builtin/agent/application/ports/sessionView";
import type { AgentProblem, AgentSessionView, Message } from "@/plugins/sdk/types/agentSessionView";
import { create } from "zustand";
import { disposeOnHmr } from "@/lib/hmr";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { reduceAgentEvent } from "@/plugins/builtin/agent/application/fold/reducer";
import { foldCancelRunResponse } from "@/plugins/builtin/agent/application/fold/cancelResponse";
import { foldRunSnapshot } from "@/plugins/builtin/agent/application/fold/runSnapshot";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import {
  dismissVisibleProblem,
  dropMessage,
  reconcileMessageIdentity,
  resolveInterrupt,
  setCommandError,
  type SettledInterrupt,
} from "@/plugins/builtin/agent/application/view/viewMutations";
import { useAgentSessionStore } from "./agentSessionStore";
import { runtimeAgentEvent, runtimeCancelResult, runtimeRunFact } from "./runtimeAgentFacts";

interface SessionEntry {
  view: AgentSessionView;
  /** Bumped before an authoritative rewrite. The useAgentSession rAF
   *  batcher stamps its queue with the epoch it saw at enqueue time and
   *  drops the batch if it changed — a flush scheduled before the replacement
   *  must not append the old run's tail events into the rebuilt view. */
  viewEpoch: number;
  /** Changes after every material projection write. Authoritative refreshes
   *  compare this revision before replacing the view, so a fetch started
   *  before a user action or live event cannot overwrite it. */
  viewRevision: number;
  /** Changes only when a durable authoritative projection commits. Command
   * recovery uses this boundary instead of mistaking a live event or local
   * optimistic write for proof of remote mutation settlement. */
  authoritativeRevision: number;
  /** Latest refresh request for this session. A newer read supersedes an older
   *  in-flight read even while the material view itself is unchanged. */
  refreshSequence: number;
  stop: StopCurrentRootRunAction | null;
  send: SendAgentInputAction | null;
  resume: ResumeRunAction | null;
  synchronize: SynchronizeSessionAction | null;
  cancelRun: CancelRunAction | null;
}

interface AgentStore {
  sessions: Record<string, SessionEntry>;
  /** Monotonic high-water mark for every mounted projection generation in this
   * renderer. Session material may be pruned, but its retired identity must
   * never become available to a later remount of the same Session. */
  projectionGenerationSequence: number;

  /**
   * Fold a batch of {event, runId} into the named session's view state with a
   * single `set()` — used by the per-frame batcher in useAgentSession so a
   * burst of streaming item.delta events produces one React commit per frame
   * instead of one per delta.
   */
  applyRunEvents: (sessionId: string, events: RunEvent[]) => void;
  applyRunSnapshot: (sessionId: string, run: RunRef) => void;
  commitCancelResponse: (
    sessionId: string,
    expected: { viewEpoch: number; viewRevision: number },
    response: CancelRunResponse,
  ) => boolean;
  appendLocalMessage: (sessionId: string, message: Message) => void;
  /** Create the mounted session slice if absent. Existing projection state is
   *  retained while a new authoritative read is in flight. */
  ensureSession: (sessionId: string) => void;
  beginViewRefresh: (
    sessionId: string,
    invalidateQueuedRunEvents: boolean,
  ) => AgentViewRefreshToken | null;
  commitViewRefresh: (
    sessionId: string,
    token: AgentViewRefreshToken,
    view: AgentSessionView,
  ) => boolean;
  retireProjectionGeneration: (sessionIds: readonly string[]) => void;
  replaceServerScope: (sessionIds: readonly string[]) => void;
  /**
   * Reconcile an optimistic placeholder with the server id named by the run
   * acknowledgement. If the streamed item won the race and already occupies
   * that id, the placeholder is collapsed into it.
   */
  reconcileMessageIdentity: (sessionId: string, fromId: string, toId: string) => void;
  /** Remove one message by id. Used to roll back an optimistic steer bubble
   *  when the run ended mid-type (run_not_found) and the send falls back to a
   *  fresh turn that mints its own bubble. No-op if the id is gone. */
  dropMessage: (sessionId: string, id: string) => void;
  /** Remove a session entry entirely (freeing its view state). */
  dropSession: (sessionId: string) => void;
  /** Bind / unbind the imperative stop action for a session. */
  setStop: (sessionId: string, action: StopCurrentRootRunAction | null) => void;
  /** Bind / unbind the imperative send action for a session. */
  setSend: (sessionId: string, action: SendAgentInputAction | null) => void;
  /** Bind / unbind the imperative HITL resume action for a session. */
  setResume: (sessionId: string, action: ResumeRunAction | null) => void;
  /** Bind / unbind authoritative query + tail synchronization. */
  setSynchronize: (sessionId: string, action: SynchronizeSessionAction | null) => void;
  /** Bind / unbind committed root-or-child Run cancellation. */
  setCancelRun: (sessionId: string, action: CancelRunAction | null) => void;
  /** Dismiss the error banner for a session without resetting the rest. */
  clearProblem: (sessionId: string) => void;
  /** Surface a channel-a failure (a rejected runs.start / runs.resume, API.md
   *  §8.1) on the run-error banner — the stream never opened, so no
   *  segment.finished{error} will arrive to carry it. */
  setCommandError: (sessionId: string, error: AgentProblem | null) => void;
  /**
   * Optimistically settle a HITL block after its complete interrupt set has
   * opened a continuation through `runs.resume`:
   * stamp the approval/question block (by interrupt itemId) + drop the
   * matching open interrupt. The continuation Run streams the real
   * follow-up; this just flips the card out of its requires-action state.
   */
  resolveInterrupt: (
    sessionId: string,
    itemId: string,
    settled: SettledInterrupt,
    resolvedAt: number,
  ) => void;
}

const emptyEntry = (viewEpoch: number): SessionEntry => ({
  view: EMPTY_AGENT_SESSION_VIEW,
  viewEpoch,
  viewRevision: 0,
  authoritativeRevision: 0,
  refreshSequence: 0,
  stop: null,
  send: null,
  resume: null,
  synchronize: null,
  cancelRun: null,
});

// Patch an EXISTING session entry. Never resurrects a dropped slice:
// ensureSession (run once at mount) is the sole creator, so a write that can't
// find its session — a late rAF flush, an in-flight snapshot resolving, or
// the unmount cleanup nulling send/stop after the prune subscriber already
// dropped the session — must no-op rather than re-seed a ghost entry that prune
// will never collect again (it only fires on the next openSessionIds change).
function patchSession(
  sessions: Record<string, SessionEntry>,
  sessionId: string,
  next: Partial<SessionEntry>,
): Record<string, SessionEntry> {
  const prev = sessions[sessionId];
  if (!prev) return sessions;
  return { ...sessions, [sessionId]: { ...prev, ...next } };
}

function patchView(
  sessions: Record<string, SessionEntry>,
  sessionId: string,
  update: (view: AgentSessionView) => AgentSessionView,
): Record<string, SessionEntry> {
  const prev = sessions[sessionId];
  if (!prev) return sessions;
  const view = update(prev.view);
  if (view === prev.view) return sessions;
  return patchSession(sessions, sessionId, {
    view,
    viewRevision: prev.viewRevision + 1,
  });
}

function patchSessionState(
  state: AgentStore,
  sessionId: string,
  next: Partial<SessionEntry>,
): AgentStore | { sessions: Record<string, SessionEntry> } {
  const sessions = patchSession(state.sessions, sessionId, next);
  return sessions === state.sessions ? state : { sessions };
}

export const useAgentStore = create<AgentStore>((set) => ({
  sessions: {},
  projectionGenerationSequence: 0,
  applyRunEvents: (sessionId, events) =>
    set((state) => {
      if (events.length === 0) return state;
      const prev = state.sessions[sessionId];
      if (!prev) return state; // session torn down — drop the late batch
      let view = prev.view;
      for (const event of events) view = reduceAgentEvent(view, runtimeAgentEvent(event));
      if (view === prev.view) return state;
      return {
        sessions: patchSession(state.sessions, sessionId, {
          view,
          viewRevision: prev.viewRevision + 1,
        }),
      };
    }),
  applyRunSnapshot: (sessionId, run) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) =>
        foldRunSnapshot(view, runtimeRunFact(run)),
      );
      return sessions === state.sessions ? state : { sessions };
    }),
  commitCancelResponse: (sessionId, expected, response) => {
    let committed = false;
    set((state) => {
      const entry = state.sessions[sessionId];
      if (
        !entry ||
        entry.viewEpoch !== expected.viewEpoch ||
        entry.viewRevision !== expected.viewRevision
      ) {
        return state;
      }
      const sessions = patchView(state.sessions, sessionId, (view) =>
        foldCancelRunResponse(view, runtimeCancelResult(response)),
      );
      committed = true;
      return sessions === state.sessions ? state : { sessions };
    });
    return committed;
  },
  appendLocalMessage: (sessionId, message) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) =>
        view.messages.some((existing) => existing.id === message.id)
          ? view
          : { ...view, messages: [...view.messages, message] },
      );
      return sessions === state.sessions ? state : { sessions };
    }),
  ensureSession: (sessionId) =>
    set((state) => {
      if (state.sessions[sessionId]) return state;
      const viewEpoch = state.projectionGenerationSequence + 1;
      return {
        sessions: { ...state.sessions, [sessionId]: emptyEntry(viewEpoch) },
        projectionGenerationSequence: viewEpoch,
      };
    }),
  beginViewRefresh: (sessionId, invalidateQueuedRunEvents) => {
    let token: AgentViewRefreshToken | null = null;
    set((state) => {
      const entry = state.sessions[sessionId];
      if (!entry) return state;
      const requestSequence = entry.refreshSequence + 1;
      const viewEpoch = invalidateQueuedRunEvents
        ? state.projectionGenerationSequence + 1
        : entry.viewEpoch;
      token = {
        generation: viewEpoch,
        requestSequence,
        viewRevision: entry.viewRevision,
      };
      const sessions = patchSession(state.sessions, sessionId, {
        refreshSequence: requestSequence,
        viewEpoch,
      });
      return {
        sessions,
        ...(invalidateQueuedRunEvents ? { projectionGenerationSequence: viewEpoch } : {}),
      };
    });
    return token;
  },
  commitViewRefresh: (sessionId, token, view) => {
    let committed = false;
    set((state) => {
      const entry = state.sessions[sessionId];
      if (
        !entry ||
        entry.viewEpoch !== token.generation ||
        entry.refreshSequence !== token.requestSequence ||
        entry.viewRevision !== token.viewRevision
      ) {
        return state;
      }
      committed = true;
      return patchSessionState(state, sessionId, {
        view,
        viewRevision: entry.viewRevision + 1,
        authoritativeRevision: entry.authoritativeRevision + 1,
      });
    });
    return committed;
  },
  retireProjectionGeneration: (sessionIds) =>
    set((state) => {
      const viewEpoch = state.projectionGenerationSequence + 1;
      let sessions = state.sessions;
      for (const sessionId of new Set(sessionIds)) {
        const entry = sessions[sessionId];
        if (!entry) continue;
        sessions = patchSession(sessions, sessionId, {
          refreshSequence: entry.refreshSequence + 1,
          viewEpoch,
        });
      }
      return sessions === state.sessions
        ? state
        : { sessions, projectionGenerationSequence: viewEpoch };
    }),
  replaceServerScope: (sessionIds) =>
    set((state) => {
      const viewEpoch = state.projectionGenerationSequence + 1;
      let sessions = state.sessions;
      for (const sessionId of new Set(sessionIds)) {
        const entry = sessions[sessionId];
        if (!entry) continue;
        sessions = patchSession(sessions, sessionId, {
          view: EMPTY_AGENT_SESSION_VIEW,
          viewEpoch,
          viewRevision: entry.viewRevision + 1,
          authoritativeRevision: 0,
          refreshSequence: entry.refreshSequence + 1,
        });
      }
      return sessions === state.sessions
        ? state
        : { sessions, projectionGenerationSequence: viewEpoch };
    }),
  reconcileMessageIdentity: (sessionId, fromId, toId) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) =>
        reconcileMessageIdentity(view, fromId, toId),
      );
      return sessions === state.sessions ? state : { sessions };
    }),
  dropMessage: (sessionId, id) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) => dropMessage(view, id));
      return sessions === state.sessions ? state : { sessions };
    }),
  dropSession: (sessionId) =>
    set((state) => {
      if (!(sessionId in state.sessions)) return state;
      const next = { ...state.sessions };
      delete next[sessionId];
      return { sessions: next };
    }),
  setStop: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { stop: action })),
  setSend: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { send: action })),
  setResume: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { resume: action })),
  setSynchronize: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { synchronize: action })),
  setCancelRun: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { cancelRun: action })),
  clearProblem: (sessionId) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, dismissVisibleProblem);
      return sessions === state.sessions ? state : { sessions };
    }),
  setCommandError: (sessionId, error) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) => setCommandError(view, error));
      return sessions === state.sessions ? state : { sessions };
    }),
  resolveInterrupt: (sessionId, itemId, settled, resolvedAt) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) =>
        resolveInterrupt(view, itemId, settled, resolvedAt),
      );
      return sessions === state.sessions ? state : { sessions };
    }),
}));

/**
 * Exact Plugin Host generation allowed to commit authoritative snapshot reads.
 *
 * Refresh sequence and view revision order writes within one generation. They cannot
 * distinguish an old port object retained across adapter replacement, because both
 * objects target this process-wide store. This owner adds that missing outer identity:
 * installing a successor synchronously revokes every token held by its predecessor,
 * and a stale disposer can retire only itself.
 */
export class AgentViewRefreshOwner {
  #disposed = false;

  private constructor() {}

  static install(): AgentViewRefreshOwner {
    const owner = new AgentViewRefreshOwner();
    agentViewRefreshPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  begin(sessionId: string, invalidateQueuedRunEvents: boolean): AgentViewRefreshToken | null {
    if (!this.#ownsGeneration()) return null;
    return useAgentStore.getState().beginViewRefresh(sessionId, invalidateQueuedRunEvents);
  }

  commit(sessionId: string, token: AgentViewRefreshToken, view: AgentSessionView): boolean {
    if (!this.#ownsGeneration()) return false;
    return useAgentStore.getState().commitViewRefresh(sessionId, token, view);
  }

  retireProjectionGeneration(sessionIds: readonly string[]): void {
    if (!this.#ownsGeneration()) return;
    useAgentStore.getState().retireProjectionGeneration(sessionIds);
  }

  replaceServerScope(sessionIds: readonly string[]): void {
    if (!this.#ownsGeneration()) return;
    useAgentStore.getState().replaceServerScope(sessionIds);
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    agentViewRefreshPublication.withdraw(this);
  }

  #ownsGeneration(): boolean {
    return !this.#disposed && agentViewRefreshPublication.owns(this);
  }
}

const agentViewRefreshPublication = createPublicationSlot<AgentViewRefreshOwner>();

// Prune sessions no longer held open. The view slice (messages, toolCalls,
// shared, plan) can be megabytes of streamed markdown per session — without
// this it accumulates forever.
const unsubPruneSessions = useAgentSessionStore.subscribe((state, prev) => {
  if (state.openSessionIds === prev.openSessionIds) return;
  const live = new Set(state.openSessionIds);
  const sessions = useAgentStore.getState().sessions;
  for (const id of Object.keys(sessions)) {
    if (!live.has(id)) useAgentStore.getState().dropSession(id);
  }
});

disposeOnHmr(unsubPruneSessions);
