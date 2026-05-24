// Per-session agent view state — each session keeps its own
// AgentViewState slice + imperative stop/send fns, so switching
// sessions feels like switching browser tabs (no event leakage when
// multiple agents stream concurrently). Read via the selector hooks
// below; they do the active-session lookup + INITIAL_VIEW_STATE
// fallback that every callsite needs.

import { create } from "zustand";
import type { BaseEvent } from "@ag-ui/core";
import { reduce } from "@/protocol/agui/reducer";
import { INITIAL_VIEW_STATE, type AgentViewState } from "@/protocol/agui/viewState";
import { useSessionStore } from "./sessionStore";

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
  /** Dismiss the error banner for a session without resetting the rest. */
  clearError: (sessionId: string) => void;
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
  clearError: (sessionId) =>
    set((s) => {
      const prev = s.sessions[sessionId];
      if (!prev) return s;
      return {
        sessions: patch(s.sessions, sessionId, {
          view: { ...prev.view, error: null },
        }),
      };
    }),
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
 * Imperative helper — read the current session's view from outside React.
 * Used by non-component code (toolRouting, plugin command handlers).
 */
export function getCurrentSessionView(): AgentViewState {
  const sid = useSessionStore.getState().activeSessionId;
  return useAgentStore.getState().sessions[sid]?.view ?? INITIAL_VIEW_STATE;
}
