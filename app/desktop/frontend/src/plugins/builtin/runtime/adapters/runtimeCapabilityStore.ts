// Runtime discovery state — the server capabilities returned by
// `runtime.discover` or `/v2/info`, so UI can gate optional features behind
// what the server actually supports.
//
// Per docs/protocol/API.md §6.1: "Frontend treats every features.* as false by
// default" — so when the store is empty (before discovery), every capability
// selector returns false. UI MUST handle that gracefully (e.g. hide a button
// instead of crashing).
//
// Separate concern from agentStore (per-session view state), uiStore (theme /
// layout / motion prefs), and agentSessionStore (session tabs). The discovery
// result is global and runtime-lifetime.

import { create } from "zustand";
import type { ServerCapabilities } from "@/rpc";

interface RuntimeState {
  /** What the server can do. Null before discovery. */
  capabilities: ServerCapabilities | null;
  /** Store discovered server capabilities. */
  setDiscovery: (capabilities: ServerCapabilities) => void;
}

export const useRuntimeStore = create<RuntimeState>((set) => ({
  capabilities: null,
  setDiscovery: (capabilities) => set({ capabilities }),
}));

// Selector hooks

// Boolean feature flags the server advertises via `capabilities.features.*`
// (API.md §9). Kept as a string-literal union (rather than `string`) so typos
// at the callsite are compile-time errors.
export type ServerFeature =
  | "multimodal"
  | "reasoning"
  | "checkpoints"
  | "git"
  | "fileWatch"
  | "lsp"
  | "codeIntel"
  | "todos"
  | "compaction"
  | "subagents"
  | "skills"
  | "mcp"
  | "sessionExport"
  | "memory"
  | "relocate"
  | "clientTools";

/**
 * Returns true iff the server has advertised this feature as enabled.
 * Returns false before discovery — UI must treat that as "feature off"
 * (don't show a button users can't actually use).
 */
export function useServerFeature(feature: ServerFeature): boolean {
  return useRuntimeStore((s) => s.capabilities?.features[feature] === true);
}

/** Imperative twin of {@link useServerFeature} for non-React call sites
 *  (palette commands, context-menu handlers, module-level wiring). Same
 *  pre-discovery default: false. */
export function serverFeature(feature: ServerFeature): boolean {
  return useRuntimeStore.getState().capabilities?.features[feature] === true;
}
