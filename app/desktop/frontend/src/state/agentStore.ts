// Per-session agent view state.
//
// Why per-session, not single global: in a real chat app the user
// expects switching sessions to feel like switching browser tabs —
// each tab keeps its own conversation, its own running tools, its
// own scroll position. A single global store would either (a) blow
// away state on switch (current behaviour pre-refactor) or (b) leak
// events across sessions (race conditions when multiple agents
// stream concurrently).
//
// Architecture borrowed from Proma's `streamingStatesAtom: Map<id,
// State>` pattern (`apps/electron/src/renderer/atoms/chat-atoms.ts`).
//
// Storage shape:
//   sessions[sessionId] = { view, stop, send }
//     - view: AgentViewState (messages / plan / toolCalls / run)
//       — reducer dispatches into this per-session slice
//     - stop / send: imperative actions bound by useAgentSession on
//       mount; plugins (status pill stop button, palette commands)
//       read them off the current-session entry without holding a
//       direct reference to the live agent.
//
// Consumers read via the selector hooks below. Pulling
// `s.sessions[id]?.view` directly works too but the selectors do
// the activeSessionId lookup + INITIAL_VIEW_STATE fallback for you,
// which is what every callsite needs.

import { create } from "zustand";
import type { BaseEvent } from "@ag-ui/core";
import { reduce } from "@/protocol/agui/reducer";
import { INITIAL_VIEW_STATE, type AgentViewState } from "@/protocol/agui/viewState";
import { useUIStore } from "./uiStore";

type StopFn = (() => void) | null;
type SendFn = ((text: string) => void) | null;

type SessionEntry = {
  view: AgentViewState;
  stop: StopFn;
  send: SendFn;
};

type AgentStore = {
  sessions: Record<string, SessionEntry>;

  /** Fold one AG-UI event into the named session's view state. */
  applyEvent: (sessionId: string, event: BaseEvent) => void;
  /** Discard a session's state and start clean (e.g. on agent re-mount). */
  resetSession: (sessionId: string) => void;
  /** Bind / unbind the imperative stop action for a session. */
  setStop: (sessionId: string, fn: StopFn) => void;
  /** Bind / unbind the imperative send action for a session. */
  setSend: (sessionId: string, fn: SendFn) => void;
};

const emptyEntry = (): SessionEntry => ({
  view: INITIAL_VIEW_STATE,
  stop: null,
  send: null,
});

function patch(
  sessions: Record<string, SessionEntry>,
  sessionId: string,
  next: Partial<SessionEntry>,
): Record<string, SessionEntry> {
  const prev = sessions[sessionId] ?? emptyEntry();
  return { ...sessions, [sessionId]: { ...prev, ...next } };
}

export const useAgentStore = create<AgentStore>((set) => ({
  sessions: {},
  applyEvent: (sessionId, event) =>
    set((s) => {
      const prev = s.sessions[sessionId] ?? emptyEntry();
      return { sessions: patch(s.sessions, sessionId, { view: reduce(prev.view, event) }) };
    }),
  resetSession: (sessionId) =>
    set((s) => ({ sessions: { ...s.sessions, [sessionId]: emptyEntry() } })),
  setStop: (sessionId, fn) =>
    set((s) => ({ sessions: patch(s.sessions, sessionId, { stop: fn }) })),
  setSend: (sessionId, fn) =>
    set((s) => ({ sessions: patch(s.sessions, sessionId, { send: fn }) })),
}));

// ---------------------------------------------------------------------------
// Selector hooks
// ---------------------------------------------------------------------------
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
  const sid = useUIStore((s) => s.activeSessionId);
  return useAgentStore((s) => {
    const view = s.sessions[sid]?.view ?? INITIAL_VIEW_STATE;
    return selector(view);
  });
}

/** Read the current session's `stop` or `send` action. */
export function useAgentAction(kind: "stop"): StopFn;
export function useAgentAction(kind: "send"): SendFn;
export function useAgentAction(kind: "stop" | "send"): StopFn | SendFn {
  const sid = useUIStore((s) => s.activeSessionId);
  return useAgentStore((s) => s.sessions[sid]?.[kind] ?? null);
}

/**
 * Imperative helper — read the current session's view from outside React.
 * Used by non-component code (toolRouting, plugin command handlers).
 */
export function getCurrentSessionView(): AgentViewState {
  const sid = useUIStore.getState().activeSessionId;
  return useAgentStore.getState().sessions[sid]?.view ?? INITIAL_VIEW_STATE;
}
