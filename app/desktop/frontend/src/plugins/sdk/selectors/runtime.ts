// Runtime / data-layer selectors — routes, agent sources, data providers,
// RPC hooks, plugin error fallback. The grab-bag of "things plugins
// register that don't belong to a specific UI surface".

import type {
  AgentSourceSpec,
  PluginErrorFallbackSpec,
  RouteSpec,
  RpcAfterResponseHook,
  RpcBeforeRequestHook,
} from "../types";
import { usePluginStore } from "../registry";
import { mapOwned } from "./_helpers";

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

/** Snapshot of all registered routes, sorted by `order`. */
export function listRoutes(): RouteSpec[] {
  return mapOwned(usePluginStore.getState().routes).sort(
    (a, b) => (a.order ?? 100) - (b.order ?? 100),
  );
}

// ---------------------------------------------------------------------------
// Agent sources
// ---------------------------------------------------------------------------

/**
 * Pick the active agent source — highest priority wins, ties broken by
 * insertion order. Returns undefined if none registered.
 */
export function pickAgentSource(): AgentSourceSpec | undefined {
  const sources = mapOwned(usePluginStore.getState().agentSources);
  if (sources.length === 0) return undefined;
  return sources.reduce((best, cur) => ((cur.priority ?? 0) > (best.priority ?? 0) ? cur : best));
}

// ---------------------------------------------------------------------------
// Data providers + RPC hooks
// ---------------------------------------------------------------------------

/**
 * Look up the fetcher for a data-provider key. Type is erased — callers
 * cast to their expected return shape. Returns undefined when nothing
 * registered (consumer hooks should throw or fall back).
 */
export function lookupDataProvider<T = unknown>(key: string): (() => Promise<T>) | undefined {
  const entry = usePluginStore.getState().dataProviders.get(key);
  return entry ? (entry.value.fetcher as () => Promise<T>) : undefined;
}

/** Snapshot of registered beforeRequest hooks in insertion order. */
export function listRpcBeforeHooks(): RpcBeforeRequestHook[] {
  return mapOwned(usePluginStore.getState().rpcBeforeRequest);
}

/** Snapshot of registered afterResponse hooks in insertion order. */
export function listRpcAfterHooks(): RpcAfterResponseHook[] {
  return mapOwned(usePluginStore.getState().rpcAfterResponse);
}

// ---------------------------------------------------------------------------
// Plugin error fallback
// ---------------------------------------------------------------------------

/**
 * Pick the highest-priority registered error fallback. Tied priorities
 * resolve by insertion order (later wins). Returns undefined when nothing
 * is registered.
 */
export function pickPluginErrorFallback(): PluginErrorFallbackSpec | undefined {
  const specs = mapOwned(usePluginStore.getState().pluginErrorFallbacks);
  if (specs.length === 0) return undefined;
  return specs.reduce((best, cur) => ((cur.priority ?? 0) >= (best.priority ?? 0) ? cur : best));
}
