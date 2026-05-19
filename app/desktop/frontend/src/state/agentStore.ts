// Agent view state — backed by Zustand so plugin renderers (settings panes,
// inspector tabs, content blocks) can subscribe to slices of it without
// having to thread props through the entire component tree.
//
// useAgentSession drives this store: it subscribes to the underlying AbstractAgent
// and pipes every BaseEvent through `reduce(state, event)` via `applyEvent`.
// Components read slices with `useAgentStore((s) => s.plan)` etc.
//
// Single-session for now. If/when we need multiple concurrent agent runs in
// one window, this graduates to a session-keyed map.

import { create } from "zustand";
import type { BaseEvent } from "@ag-ui/core";
import { reduce } from "@/protocol/agui/reducer";
import { INITIAL_VIEW_STATE, type AgentViewState } from "@/protocol/agui/viewState";

type AgentActions = {
  /** Fold one AG-UI event into the current state. */
  applyEvent: (event: BaseEvent) => void;
  /** Discard everything and start over (e.g. on session switch). */
  reset: () => void;
  /**
   * Imperative stop binding. `useAgentSession` plugs the live agent's
   * `abortRun` here on mount so any plugin (status-pill stop button, slash
   * command, palette command) can stop the run without prop-drilling.
   */
  stop: (() => void) | null;
  setStop: (fn: (() => void) | null) => void;
  /**
   * Same pattern as `stop` but for queuing a user message. Plugins (send
   * button, palette command, slash command) can fire a message into the
   * active agent without holding a reference to it.
   */
  send: ((text: string) => void) | null;
  setSend: (fn: ((text: string) => void) | null) => void;
};

export const useAgentStore = create<AgentViewState & AgentActions>((set) => ({
  ...INITIAL_VIEW_STATE,
  applyEvent: (event) => set((s) => reduce(s, event)),
  reset: () => set(INITIAL_VIEW_STATE),
  stop: null,
  setStop: (fn) => set({ stop: fn }),
  send: null,
  setSend: (fn) => set({ send: fn }),
}));
