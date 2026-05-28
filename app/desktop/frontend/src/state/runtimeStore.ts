// Runtime handshake state — populated after `runtime.initialize` returns.
// Holds the negotiated protocol version + server capabilities so UI can
// gate optional features behind what the server actually supports.
//
// Per docs/API.md §6.1: "Frontend treats every features.* as false by
// default" — so when the store is empty (pre-handshake), every
// capability selector returns false. UI MUST handle that gracefully
// (e.g. hide a button instead of crashing).
//
// This is a separate concern from agentStore (per-session view state),
// uiStore (theme / layout / motion prefs), and sessionStore (tab state).
// The handshake result is global, runtime-lifetime, and reset only on
// reconnect.

import { create } from "zustand";
import type { ServerCapabilities, ServerInfo } from "@/rpc";

interface RuntimeState {
  /** Server identity from `runtime.initialize` (or null pre-handshake). */
  serverInfo: ServerInfo | null;
  /** Negotiated protocol version (e.g. "2026-05-28"). */
  protocolVersion: string | null;
  /** What the server can do. Null before handshake. */
  capabilities: ServerCapabilities | null;
  /**
   * Mark handshake complete with the InitializeResult payload. Callers
   * typically pass the value straight from `methods.runtime.initialize`.
   */
  setHandshake: (result: {
    protocolVersion: string;
    serverInfo: { name: string; version: string };
    capabilities: ServerCapabilities;
  }) => void;
  /** Drop the handshake (on disconnect / reconnect / shutdown). */
  clear: () => void;
}

export const useRuntimeStore = create<RuntimeState>((set) => ({
  serverInfo: null,
  protocolVersion: null,
  capabilities: null,
  setHandshake: (result) =>
    set({
      serverInfo: {
        serverInfo: result.serverInfo,
        protocolVersion: result.protocolVersion,
        capabilities: {
          events: result.capabilities.events,
          features: result.capabilities.features,
          providers: result.capabilities.providers,
        },
      },
      protocolVersion: result.protocolVersion,
      capabilities: result.capabilities,
    }),
  clear: () => set({ serverInfo: null, protocolVersion: null, capabilities: null }),
}));

// ---------------------------------------------------------------------------
// Selector hooks
// ---------------------------------------------------------------------------

// Feature flags the server can advertise via `capabilities.features.*`.
// Keeping this as a string-literal union (rather than `string`) means
// typos at the callsite are compile-time errors.
export type ServerFeature =
  | "multimodal"
  | "reasoning"
  | "checkpoints"
  | "interrupts"
  | "background"
  | "subagents"
  | "skills"
  | "mcp"
  | "sessionExport";

/**
 * Returns true iff the server has advertised this feature as enabled.
 * Returns false pre-handshake — UI must treat that as "feature off"
 * (don't show a button users can't actually use).
 */
export function useServerFeature(feature: ServerFeature): boolean {
  return useRuntimeStore((s) => s.capabilities?.features[feature] === true);
}

/** Returns true if the server emits a specific AG-UI standard event. */
export function useServerEmitsStandard(eventType: string): boolean {
  return useRuntimeStore((s) => s.capabilities?.events.standard.includes(eventType) === true);
}

/** Returns true if the server emits a specific Lyra CUSTOM event. */
export function useServerEmitsCustom(eventName: string): boolean {
  return useRuntimeStore((s) => s.capabilities?.events.custom.includes(eventName) === true);
}

/** Returns true if the named provider is registered server-side. */
export function useServerHasProvider(providerId: string): boolean {
  return useRuntimeStore((s) => s.capabilities?.providers.includes(providerId) === true);
}
