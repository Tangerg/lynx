// Per-session agent view state — each session keeps its own
// AgentViewState slice + imperative stop/send fns, so switching
// sessions feels like switching browser tabs (no event leakage when
// multiple agents stream concurrently). Read via the selector hooks
// below; they do the active-session lookup + INITIAL_VIEW_STATE
// fallback that every callsite needs.

import type { ContentBlock, InterruptResponse, RunId, StreamEvent } from "@/rpc";
import type {
  AgentViewState,
  Message,
  PlanItem,
  RunError,
  TimelineEntry,
  ToolCall,
} from "@/protocol/run/viewState";
import { create } from "zustand";
import { disposeOnHmr } from "@/lib/hmr";
// Import from the specific SDK module, not the barrel — the barrel re-exports
// useSharedState (which reads this store), so a barrel import here would close
// a state → sdk → state module cycle.
import { appendTimelineEntry } from "@/plugins/sdk/state";
import { reduce } from "@/protocol/run/reducer";
import { INITIAL_VIEW_STATE } from "@/protocol/run/viewState";
import { useSessionStore } from "./sessionStore";

type StopFn = (() => void) | null;
type SendFn = ((input: ContentBlock[]) => void) | null;
// onSettled fires once the continuation run has actually started (channel-a
// accepted); onStartError fires if runs.resume rejects before any stream
// opened (API.md §8.1), so the caller can roll back its optimistic UI.
type ResumeFn =
  | ((
      parentRunId: RunId,
      responses: InterruptResponse[],
      onSettled?: () => void,
      onStartError?: () => void,
    ) => void)
  | null;

interface SessionEntry {
  view: AgentViewState;
  /** Bumped by resetView (rollback re-hydration). The useAgentSession rAF
   *  batcher stamps its queue with the epoch it saw at enqueue time and
   *  drops the batch if it changed — a flush scheduled before the reset
   *  must not append the old run's tail events into the rebuilt view. */
  viewEpoch: number;
  stop: StopFn;
  send: SendFn;
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
  /** Remove a session entry entirely (closing the tab — frees view state). */
  dropSession: (sessionId: string) => void;
  /** Bind / unbind the imperative stop action for a session. */
  setStop: (sessionId: string, fn: StopFn) => void;
  /** Bind / unbind the imperative send action for a session. */
  setSend: (sessionId: string, fn: SendFn) => void;
  /** Bind / unbind the imperative HITL resume action for a session. */
  setResume: (sessionId: string, fn: ResumeFn) => void;
  /** Dismiss the error banner for a session without resetting the rest. */
  clearError: (sessionId: string) => void;
  /** Surface a channel-a failure (a rejected runs.start / runs.resume, API.md
   *  §8.1) on the run-error banner — the stream never opened, so no
   *  run.finished{error} will arrive to carry it. */
  setError: (sessionId: string, error: RunError | null) => void;
  /**
   * Optimistically settle a HITL block after its `runs.resume` is sent:
   * stamp the approval/question block (by interrupt itemId) + drop the
   * matching open interrupt. The continuation Run streams the real
   * follow-up; this just flips the card out of its requires-action state.
   */
  resolveInterrupt: (
    sessionId: string,
    itemId: string,
    settled: { decision?: "approved" | "declined"; answered?: boolean },
  ) => void;
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
    set((s) => ({
      sessions: patchSession(s.sessions, sessionId, {
        view: emptyEntry().view,
        viewEpoch: (s.sessions[sessionId]?.viewEpoch ?? 0) + 1,
      }),
    })),
  relabelMessage: (sessionId, fromId, toId) =>
    set((s) => {
      const prev = s.sessions[sessionId];
      if (!prev || fromId === toId) return s;
      const msgs = prev.view.messages;
      const has = (id: string) => msgs.some((m) => m.id === id);
      // Nothing to rename, or the streamed item already landed under `toId`.
      if (!has(fromId) || has(toId)) return s;
      const messages = msgs.map((m) => (m.id === fromId ? { ...m, id: toId } : m));
      return {
        sessions: patchSession(s.sessions, sessionId, { view: { ...prev.view, messages } }),
      };
    }),
  dropSession: (sessionId) =>
    set((s) => {
      if (!(sessionId in s.sessions)) return s;
      const next = { ...s.sessions };
      delete next[sessionId];
      return { sessions: next };
    }),
  setStop: (sessionId, fn) =>
    set((s) => ({ sessions: patchSession(s.sessions, sessionId, { stop: fn }) })),
  setSend: (sessionId, fn) =>
    set((s) => ({ sessions: patchSession(s.sessions, sessionId, { send: fn }) })),
  setResume: (sessionId, fn) =>
    set((s) => ({ sessions: patchSession(s.sessions, sessionId, { resume: fn }) })),
  clearError: (sessionId) =>
    set((s) => {
      const prev = s.sessions[sessionId];
      if (!prev) return s;
      return {
        sessions: patchSession(s.sessions, sessionId, {
          view: { ...prev.view, error: null },
        }),
      };
    }),
  setError: (sessionId, error) =>
    set((s) => {
      const prev = s.sessions[sessionId];
      if (!prev) return s;
      return { sessions: patchSession(s.sessions, sessionId, { view: { ...prev.view, error } }) };
    }),
  resolveInterrupt: (sessionId, itemId, settled) =>
    set((s) => {
      const prev = s.sessions[sessionId];
      if (!prev) return s;
      const view = prev.view;
      const messages = view.messages.map((m) => {
        if (!m.blocks.some((b) => "itemId" in b && b.itemId === itemId)) return m;
        return {
          ...m,
          blocks: m.blocks.map((b) => {
            if (!("itemId" in b) || b.itemId !== itemId) return b;
            if (b.kind === "approval")
              return { ...b, status: "complete" as const, decision: settled.decision };
            if (b.kind === "question")
              return { ...b, status: "complete" as const, answered: settled.answered ?? true };
            return b;
          }),
        };
      });
      // Drop only the resolved interrupt — a single run.finished{interrupt}
      // can carry several (multiple approvals/questions at once). Removing the
      // whole envelope would strand its sibling interrupts: their cards stay
      // in requires-action with no backing open-interrupt to resume. Keep the
      // envelope until its last interrupt is resolved.
      const openInterrupts = view.openInterrupts
        .map((oi) => ({ ...oi, interrupts: oi.interrupts.filter((i) => i.itemId !== itemId) }))
        .filter((oi) => oi.interrupts.length > 0);
      let next: AgentViewState = { ...view, messages, openInterrupts };
      // Stamp the human decision on the audit timeline so the run digest +
      // Timeline view can pair it with the originating approval-request
      // (questions have no timeline counterpart — only approvals are paired).
      if (settled.decision) {
        next = appendTimelineEntry({
          kind: "approval-result",
          refId: itemId,
          status: settled.decision,
        })(next);
      }
      return { sessions: patchSession(s.sessions, sessionId, { view: next }) };
    }),
}));

// Prune sessions whose tab is closed. The view slice (messages, toolCalls,
// shared, plan) can be megabytes of streamed markdown per session — without
// this it accumulates forever.
const unsubPruneSessions = useSessionStore.subscribe((state, prev) => {
  if (state.tabIds === prev.tabIds) return;
  const live = new Set(state.tabIds);
  const sessions = useAgentStore.getState().sessions;
  for (const id of Object.keys(sessions)) {
    if (!live.has(id)) useAgentStore.getState().dropSession(id);
  }
});

disposeOnHmr(unsubPruneSessions);

// Selector hooks

//
// Components don't see the sessions map directly. They pick a slice of the
// current session's view via `useAgentSlice(v => v.messages)` etc., which
// (a) auto-tracks activeSessionId and (b) falls back to INITIAL_VIEW_STATE
// when the entry doesn't exist yet (first paint of a fresh session).
//
// useAgentAction is the equivalent for `stop` and `send`, returned as a
// function reference so callers can invoke them imperatively.

/**
 * Pick a slice of the *currently active* session's view state.
 * Returns INITIAL_VIEW_STATE-derived value when the session hasn't been
 * seeded yet (no events received).
 */
export function useAgentSlice<T>(selector: (view: AgentViewState) => T): T {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => {
    const view = s.sessions[sid]?.view ?? INITIAL_VIEW_STATE;
    return selector(view);
  });
}

/** Read the current session's `stop` or `send` action. */
export function useAgentAction(kind: "stop"): StopFn;
export function useAgentAction(kind: "send"): SendFn;
export function useAgentAction(kind: "stop" | "send"): StopFn | SendFn {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.[kind] ?? null);
}

/**
 * Granular hooks — subscribe to individual run/view fields so token-stream
 * deltas don't re-render every consumer of the `run` object. Each hook reads
 * only the scalar field it needs; Zustand's default reference equality
 * prevents re-renders when unrelated siblings change.
 */

/** Whether the active session has a run in progress. */
export function useAgentRunning(): boolean {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.view.run.running ?? false);
}

/** The active session's run id (null when idle). */
export function useAgentRunId(): string | null {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.view.run.runId ?? null);
}

/** Token usage for the active run ({ used, total }). */
export function useAgentRunTokens(): { used: string; total: string } {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.view.run.tokens ?? { used: "0", total: "0" });
}

/** Context-fill percentage for the active run (0–100). */
export function useAgentRunCtxPct(): number {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.view.run.ctxPct ?? 0);
}

/** The active session's plan items. */
export function useAgentPlan(): PlanItem[] {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.view.plan ?? []);
}

/** The active session's tool calls map. */
export function useAgentToolCalls(): Record<string, ToolCall> {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.view.toolCalls ?? {});
}

/** The active session's messages array. */
export function useAgentMessages(): Message[] {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.view.messages ?? []);
}

/** The active session's timeline entries. */
export function useAgentTimeline(): TimelineEntry[] {
  const sid = useSessionStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.view.timeline ?? []);
}

/**
 * Imperative helper — read the current session's view from outside React.
 * Used by non-component code (toolRouting, plugin command handlers).
 */
export function getCurrentSessionView(): AgentViewState {
  const sid = useSessionStore.getState().activeSessionId;
  return useAgentStore.getState().sessions[sid]?.view ?? INITIAL_VIEW_STATE;
}
