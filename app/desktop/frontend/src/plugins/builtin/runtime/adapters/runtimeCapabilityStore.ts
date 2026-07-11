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
import { configureRuntimeCapabilityPort } from "../application/ports/capabilities";
import type { RuntimeCapability } from "../domain/capability";

interface RuntimeState {
  /** What the server can do. Null before discovery. */
  capabilities: ServerCapabilities | null;
  /** Store discovered server capabilities. */
  replace: (capabilities: ServerCapabilities) => void;
  clear: () => void;
}

export const useRuntimeStore = create<RuntimeState>((set) => ({
  capabilities: null,
  replace: (capabilities) => set({ capabilities }),
  clear: () => set({ capabilities: null }),
}));

// Selector hooks

/**
 * Returns true iff the server has advertised this feature as enabled.
 * Returns false before discovery — UI must treat that as "feature off"
 * (don't show a button users can't actually use).
 */
export function useServerFeature(feature: RuntimeCapability): boolean {
  return useRuntimeStore((s) => s.capabilities?.features[feature] === true);
}

/** Imperative twin of {@link useServerFeature} for non-React call sites
 *  (palette commands, context-menu handlers, module-level wiring). Same
 *  pre-discovery default: false. */
export function serverFeature(feature: RuntimeCapability): boolean {
  return useRuntimeStore.getState().capabilities?.features[feature] === true;
}

export function runtimeSupportsStreamingMethod(method: string): boolean {
  return useRuntimeStore.getState().capabilities?.streamingMethods?.includes(method) ?? false;
}

export function subscribeRuntimeCapabilities(onChange: () => void): () => void {
  return useRuntimeStore.subscribe(onChange);
}

export function installRuntimeCapabilityPort(): void {
  configureRuntimeCapabilityPort({
    useCapability: useServerFeature,
    hasCapability: serverFeature,
    supportsStreamingMethod: runtimeSupportsStreamingMethod,
    subscribe: subscribeRuntimeCapabilities,
    replace: (capabilities) => useRuntimeStore.getState().replace(capabilities),
    clear: () => useRuntimeStore.getState().clear(),
  });
}
