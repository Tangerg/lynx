// Runtime handshake state — populated after `runtime.initialize` returns.
// Holds the negotiated protocol version + server capabilities so UI can
// gate optional features behind what the server actually supports.
//
// Per docs/protocol/API.md §6.1: "Frontend treats every features.* as false by
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
  /** Server name + version (`{name, version}` from initialize result). */
  serverName: string | null;
  serverVersion: string | null;
  /** Negotiated protocol version (e.g. "2026-06-03"). */
  protocolVersion: string | null;
  /** What the server can do. Null before handshake. */
  capabilities: ServerCapabilities | null;
  /**
   * Mark handshake complete with the InitializeResponse payload. Callers
   * typically pass the value straight from `methods.runtime.initialize`.
   */
  setHandshake: (result: {
    protocolVersion: string;
    serverInfo: ServerInfo;
    capabilities: ServerCapabilities;
  }) => void;
  /** Drop the handshake (on disconnect / reconnect / shutdown). */
  clear: () => void;
}

export const useRuntimeStore = create<RuntimeState>((set) => ({
  serverName: null,
  serverVersion: null,
  protocolVersion: null,
  capabilities: null,
  setHandshake: (result) =>
    set({
      serverName: result.serverInfo.name,
      serverVersion: result.serverInfo.version,
      protocolVersion: result.protocolVersion,
      capabilities: result.capabilities,
    }),
  clear: () =>
    set({
      serverName: null,
      serverVersion: null,
      protocolVersion: null,
      capabilities: null,
    }),
}));

// ---------------------------------------------------------------------------
// Selector hooks
// ---------------------------------------------------------------------------

// Feature flags the server can advertise via `capabilities.features.*`.
// Keeping this as a string-literal union (rather than `string`) means
// typos at the callsite are compile-time errors.
// Boolean feature flags the server advertises via `capabilities.features.*`
// (API.md §9). `attachments` is an object, not a boolean, so it's excluded
// here — read `capabilities.features.attachments` directly when needed.
export type ServerFeature =
  | "multimodal"
  | "reasoning"
  | "checkpoints"
  | "git"
  | "fileWatch"
  | "lsp"
  | "subagents"
  | "skills"
  | "mcp"
  | "sessionExport"
  | "memory"
  | "relocate"
  | "clientTools";

/**
 * Returns true iff the server has advertised this feature as enabled.
 * Returns false pre-handshake — UI must treat that as "feature off"
 * (don't show a button users can't actually use).
 */
export function useServerFeature(feature: ServerFeature): boolean {
  return useRuntimeStore((s) => s.capabilities?.features[feature] === true);
}

/** Imperative twin of {@link useServerFeature} for non-React call sites
 *  (palette commands, context-menu handlers, module-level wiring). Same
 *  pre-handshake default: false. */
export function serverFeature(feature: ServerFeature): boolean {
  return useRuntimeStore.getState().capabilities?.features[feature] === true;
}

/** Returns true if the server emits a specific StreamEvent type (§9). */
export function useServerEmitsEvent(eventType: string): boolean {
  return useRuntimeStore((s) => s.capabilities?.events.includes(eventType) === true);
}

/** Returns true if the named provider is registered server-side. */
export function useServerHasProvider(providerId: string): boolean {
  return useRuntimeStore((s) => s.capabilities?.providers.includes(providerId) === true);
}
