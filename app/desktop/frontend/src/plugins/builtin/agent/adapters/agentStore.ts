import type { AgentRunStartOptions } from "@/plugins/sdk/types";
import type { StreamEvent } from "@/rpc";
import type { AgentInput } from "@/plugins/builtin/agent/domain/input";
import type { ResumeFn } from "@/plugins/builtin/agent/application/ports/viewState";
import type { AgentViewState, RunError } from "@/plugins/builtin/agent/public/viewState";
import { create } from "zustand";
import { disposeOnHmr } from "@/lib/hmr";
import { reduce } from "@/plugins/builtin/agent/application/fold/reducer";
import { INITIAL_VIEW_STATE } from "@/plugins/builtin/agent/public/viewState";
import {
  cancelRunningRun,
  dropMessage,
  relabelMessage,
  resolveInterrupt,
  setRunError,
  type SettledInterrupt,
} from "@/plugins/builtin/agent/application/view/viewMutations";
import { useAgentSessionStore } from "./agentSessionStore";

export type AgentStopAction = (() => void) | null;
export type AgentSendAction = ((input: AgentInput, options?: AgentRunStartOptions) => void) | null;

interface SessionEntry {
  view: AgentViewState;
  /** Bumped by resetView (rollback re-hydration). The useAgentSession rAF
   *  batcher stamps its queue with the epoch it saw at enqueue time and
   *  drops the batch if it changed — a flush scheduled before the reset
   *  must not append the old run's tail events into the rebuilt view. */
  viewEpoch: number;
  stop: AgentStopAction;
  send: AgentSendAction;
  resume: ResumeFn;
}

/** A StreamEvent + the wire (envelope) runId of the RunEvent that carried it —
 *  the fold needs the runId to tell a subagent's run.* from the root run's
 *  (RunOutcome itself has no id). Absent for synthetic events (the optimistic
 *  local user bubble, items.list history replay). */
export interface FoldEvent {
  event: StreamEvent;
  runId?: string;
}

interface AgentStore {
  sessions: Record<string, SessionEntry>;

  /** Fold one StreamEvent into the named session's view state. `runId` is the
   *  wire envelope runId (subagent discrimination); omit for synthetic events. */
  applyEvent: (sessionId: string, event: StreamEvent, runId?: string) => void;
  /**
   * Fold a batch of {event, runId} into the named session's view state with a
   * single `set()` — used by the per-frame batcher in useAgentSession so a
   * burst of streaming item.delta events produces one React commit per frame
   * instead of one per delta.
   */
  applyEvents: (sessionId: string, events: FoldEvent[]) => void;
  /** Discard a session's state and start clean (e.g. on agent re-mount). */
  resetSession: (sessionId: string) => void;
  resetView: (sessionId: string) => void;
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
  /** Remove a session entry entirely (closing the tab — frees view state). */
  dropSession: (sessionId: string) => void;
  /** Bind / unbind the imperative stop action for a session. */
  setStop: (sessionId: string, fn: AgentStopAction) => void;
  /** Bind / unbind the imperative send action for a session. */
  setSend: (sessionId: string, fn: AgentSendAction) => void;
  /** Bind / unbind the imperative HITL resume action for a session. */
  setResume: (sessionId: string, fn: ResumeFn) => void;
  /** Dismiss the error banner for a session without resetting the rest. */
  clearError: (sessionId: string) => void;
  /** Surface a channel-a failure (a rejected runs.start / runs.resume, API.md
   *  §8.1) on the run-error banner — the stream never opened, so no
   *  run.finished{error} will arrive to carry it. */
  setError: (sessionId: string, error: RunError | null) => void;
  /**
   * Locally settle a user-stopped run. `stop()` aborts the event stream, which
   * closes the channel BEFORE the backend's run.finished{canceled} can reach
   * the fold — so flip `running` off here (and stamp a canceled timeline entry)
   * or the view stays stuck "running": the status bar spins, the composer's
   * Stop button stays latched, and useChatSend's `running` guard blocks the
   * next send until a remount. Preserves the run's token/step readout (a
   * synthetic run.finished would zero it). No-op once the run has settled.
   */
  cancelRun: (sessionId: string) => void;
  /**
   * Optimistically settle a HITL block after its `runs.resume` is sent:
   * stamp the approval/question block (by interrupt itemId) + drop the
   * matching open interrupt. The continuation Run streams the real
   * follow-up; this just flips the card out of its requires-action state.
   */
  resolveInterrupt: (sessionId: string, itemId: string, settled: SettledInterrupt) => void;
}

const emptyEntry = (): SessionEntry => ({
  view: INITIAL_VIEW_STATE,
  viewEpoch: 0,
  stop: null,
  send: null,
  resume: null,
});

// Patch an EXISTING session entry. Never resurrects a dropped slice:
// resetSession (run once at mount) is the sole creator, so a write that can't
// find its session — a late rAF flush, an in-flight items.list resolving, or
// the unmount cleanup nulling send/stop after the prune subscriber already
// dropped the tab — must no-op rather than re-seed a ghost entry that prune
// will never collect again (it only fires on the next tabIds change).
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
  update: (view: AgentViewState) => AgentViewState,
): Record<string, SessionEntry> {
  const prev = sessions[sessionId];
  if (!prev) return sessions;
  const view = update(prev.view);
  if (view === prev.view) return sessions;
  return patchSession(sessions, sessionId, { view });
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
  applyEvent: (sessionId, event, runId) =>
    set((s) => {
      const prev = s.sessions[sessionId];
      if (!prev) return s; // session torn down — drop the late event
      return {
        sessions: patchSession(s.sessions, sessionId, { view: reduce(prev.view, event, runId) }),
      };
    }),
  applyEvents: (sessionId, events) =>
    set((s) => {
      if (events.length === 0) return s;
      const prev = s.sessions[sessionId];
      if (!prev) return s; // session torn down — drop the late batch
      let view = prev.view;
      for (const { event, runId } of events) view = reduce(view, event, runId);
      return { sessions: patchSession(s.sessions, sessionId, { view }) };
    }),
  resetSession: (sessionId) =>
    set((s) => ({ sessions: { ...s.sessions, [sessionId]: emptyEntry() } })),
  // Reset ONLY the view, keeping the mounted session's send/stop/resume
  // bindings — for external re-hydration (after sessions.rollback the server
  // history shrank; the view rebuilds from items.list while the composer
  // must keep working without a remount).
  resetView: (sessionId) =>
    set((s) =>
      patchSessionState(s, sessionId, {
        view: emptyEntry().view,
        viewEpoch: (s.sessions[sessionId]?.viewEpoch ?? 0) + 1,
      }),
    ),
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
  clearError: (sessionId) =>
    set((s) => {
      const sessions = patchView(s.sessions, sessionId, (view) => setRunError(view, null));
      return sessions === s.sessions ? s : { sessions };
    }),
  setError: (sessionId, error) =>
    set((s) => {
      const sessions = patchView(s.sessions, sessionId, (view) => setRunError(view, error));
      return sessions === s.sessions ? s : { sessions };
    }),
  cancelRun: (sessionId) =>
    set((s) => {
      const sessions = patchView(s.sessions, sessionId, cancelRunningRun);
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

// Prune sessions whose tab is closed. The view slice (messages, toolCalls,
// shared, plan) can be megabytes of streamed markdown per session — without
// this it accumulates forever.
const unsubPruneSessions = useAgentSessionStore.subscribe((state, prev) => {
  if (state.tabIds === prev.tabIds) return;
  const live = new Set(state.tabIds);
  const sessions = useAgentStore.getState().sessions;
  for (const id of Object.keys(sessions)) {
    if (!live.has(id)) useAgentStore.getState().dropSession(id);
  }
});

disposeOnHmr(unsubPruneSessions);
