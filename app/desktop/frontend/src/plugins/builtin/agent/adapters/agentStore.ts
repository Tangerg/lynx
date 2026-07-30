import type { AgentRunStartOptions } from "@/plugins/sdk/types";
import type { CancelRunResponse, RunEvent } from "@/rpc";
import type { AgentInput } from "@/plugins/builtin/agent/domain/input";
import type {
  AgentViewRefreshToken,
  CancelRunFn,
  ResumeFn,
  SynchronizeFn,
} from "@/plugins/builtin/agent/application/ports/sessionView";
import type { AgentProblem, AgentSessionView, Message } from "@/plugins/sdk/types/agentSessionView";
import { create } from "zustand";
import { disposeOnHmr } from "@/lib/hmr";
import { reduceRunEvent } from "@/plugins/builtin/agent/application/fold/reducer";
import { foldCancelRunResponse } from "@/plugins/builtin/agent/application/fold/cancelResponse";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import {
  dismissVisibleProblem,
  dropMessage,
  relabelMessage,
  resolveInterrupt,
  setCommandError,
  type SettledInterrupt,
} from "@/plugins/builtin/agent/application/view/viewMutations";
import { useAgentSessionStore } from "./agentSessionStore";

export type AgentStopAction = (() => void) | null;
export type AgentSendAction = ((input: AgentInput, options?: AgentRunStartOptions) => void) | null;

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
  /** Latest refresh request for this session. A newer read supersedes an older
   *  in-flight read even while the material view itself is unchanged. */
  refreshSequence: number;
  stop: AgentStopAction;
  send: AgentSendAction;
  resume: ResumeFn;
  synchronize: SynchronizeFn;
  cancelRun: CancelRunFn;
}

interface AgentStore {
  sessions: Record<string, SessionEntry>;

  /**
   * Fold a batch of {event, runId} into the named session's view state with a
   * single `set()` — used by the per-frame batcher in useAgentSession so a
   * burst of streaming item.delta events produces one React commit per frame
   * instead of one per delta.
   */
  applyRunEvents: (sessionId: string, events: RunEvent[]) => void;
  applyCancelResponse: (sessionId: string, response: CancelRunResponse) => void;
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
  /**
   * Rename a message id (optimistic placeholder → server id). Used to
   * reconcile the optimistic user bubble with the run's `userItemId` the
   * moment runs.start resolves, so the streamed userMessage Item dedupes by
   * exact id. No-op if `fromId` is gone or `toId` already exists (the streamed
   * item won).
   */
  relabelMessage: (sessionId: string, fromId: string, toId: string) => void;
  /** Remove one message by id. Used to roll back an optimistic steer bubble
   *  when the run ended mid-type (run_not_found) and the send falls back to a
   *  fresh turn that mints its own bubble. No-op if the id is gone. */
  dropMessage: (sessionId: string, id: string) => void;
  /** Remove a session entry entirely (freeing its view state). */
  dropSession: (sessionId: string) => void;
  /** Bind / unbind the imperative stop action for a session. */
  setStop: (sessionId: string, fn: AgentStopAction) => void;
  /** Bind / unbind the imperative send action for a session. */
  setSend: (sessionId: string, fn: AgentSendAction) => void;
  /** Bind / unbind the imperative HITL resume action for a session. */
  setResume: (sessionId: string, fn: ResumeFn) => void;
  /** Bind / unbind authoritative query + tail synchronization. */
  setSynchronize: (sessionId: string, fn: SynchronizeFn) => void;
  /** Bind / unbind committed root-or-child Run cancellation. */
  setCancelRun: (sessionId: string, fn: CancelRunFn) => void;
  /** Dismiss the error banner for a session without resetting the rest. */
  clearProblem: (sessionId: string) => void;
  /** Surface a channel-a failure (a rejected runs.start / runs.resume, API.md
   *  §8.1) on the run-error banner — the stream never opened, so no
   *  segment.finished{error} will arrive to carry it. */
  setCommandError: (sessionId: string, error: AgentProblem | null) => void;
  /**
   * Optimistically settle a HITL block after its `runs.resume` is sent:
   * stamp the approval/question block (by interrupt itemId) + drop the
   * matching open interrupt. The continuation Run streams the real
   * follow-up; this just flips the card out of its requires-action state.
   */
  resolveInterrupt: (sessionId: string, itemId: string, settled: SettledInterrupt) => void;
}

const emptyEntry = (): SessionEntry => ({
  view: EMPTY_AGENT_SESSION_VIEW,
  viewEpoch: 0,
  viewRevision: 0,
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
  applyRunEvents: (sessionId, events) =>
    set((s) => {
      if (events.length === 0) return s;
      const prev = s.sessions[sessionId];
      if (!prev) return s; // session torn down — drop the late batch
      let view = prev.view;
      for (const event of events) view = reduceRunEvent(view, event);
      if (view === prev.view) return s;
      return {
        sessions: patchSession(s.sessions, sessionId, {
          view,
          viewRevision: prev.viewRevision + 1,
        }),
      };
    }),
  applyCancelResponse: (sessionId, response) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) =>
        foldCancelRunResponse(view, response),
      );
      return sessions === state.sessions ? state : { sessions };
    }),
  appendLocalMessage: (sessionId, message) =>
    set((s) => {
      const sessions = patchView(s.sessions, sessionId, (view) =>
        view.messages.some((existing) => existing.id === message.id)
          ? view
          : { ...view, messages: [...view.messages, message] },
      );
      return sessions === s.sessions ? s : { sessions };
    }),
  ensureSession: (sessionId) =>
    set((state) =>
      state.sessions[sessionId]
        ? state
        : { sessions: { ...state.sessions, [sessionId]: emptyEntry() } },
    ),
  beginViewRefresh: (sessionId, invalidateQueuedRunEvents) => {
    let token: AgentViewRefreshToken | null = null;
    set((state) => {
      const entry = state.sessions[sessionId];
      if (!entry) return state;
      const requestSequence = entry.refreshSequence + 1;
      token = {
        requestSequence,
        viewRevision: entry.viewRevision,
      };
      return patchSessionState(state, sessionId, {
        refreshSequence: requestSequence,
        ...(invalidateQueuedRunEvents ? { viewEpoch: entry.viewEpoch + 1 } : {}),
      });
    });
    return token;
  },
  commitViewRefresh: (sessionId, token, view) => {
    let committed = false;
    set((state) => {
      const entry = state.sessions[sessionId];
      if (
        !entry ||
        entry.refreshSequence !== token.requestSequence ||
        entry.viewRevision !== token.viewRevision
      ) {
        return state;
      }
      committed = true;
      return patchSessionState(state, sessionId, {
        view,
        viewRevision: entry.viewRevision + 1,
      });
    });
    return committed;
  },
  relabelMessage: (sessionId, fromId, toId) =>
    set((s) => {
      const sessions = patchView(s.sessions, sessionId, (view) =>
        relabelMessage(view, fromId, toId),
      );
      return sessions === s.sessions ? s : { sessions };
    }),
  dropMessage: (sessionId, id) =>
    set((s) => {
      const sessions = patchView(s.sessions, sessionId, (view) => dropMessage(view, id));
      return sessions === s.sessions ? s : { sessions };
    }),
  dropSession: (sessionId) =>
    set((s) => {
      if (!(sessionId in s.sessions)) return s;
      const next = { ...s.sessions };
      delete next[sessionId];
      return { sessions: next };
    }),
  setStop: (sessionId, fn) => set((s) => patchSessionState(s, sessionId, { stop: fn })),
  setSend: (sessionId, fn) => set((s) => patchSessionState(s, sessionId, { send: fn })),
  setResume: (sessionId, fn) => set((s) => patchSessionState(s, sessionId, { resume: fn })),
  setSynchronize: (sessionId, fn) =>
    set((s) => patchSessionState(s, sessionId, { synchronize: fn })),
  setCancelRun: (sessionId, fn) => set((s) => patchSessionState(s, sessionId, { cancelRun: fn })),
  clearProblem: (sessionId) =>
    set((s) => {
      const sessions = patchView(s.sessions, sessionId, dismissVisibleProblem);
      return sessions === s.sessions ? s : { sessions };
    }),
  setCommandError: (sessionId, error) =>
    set((s) => {
      const sessions = patchView(s.sessions, sessionId, (view) => setCommandError(view, error));
      return sessions === s.sessions ? s : { sessions };
    }),
  resolveInterrupt: (sessionId, itemId, settled) =>
    set((s) => {
      const sessions = patchView(s.sessions, sessionId, (view) =>
        resolveInterrupt(view, itemId, settled),
      );
      return sessions === s.sessions ? s : { sessions };
    }),
}));

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
