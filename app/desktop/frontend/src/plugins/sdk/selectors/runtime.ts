// Runtime / data-layer selectors — routes, agent sources, data providers,
// RPC hooks, plugin error fallback. The grab-bag of "things plugins
// register that don't belong to a specific UI surface".

import type {
  AgentSourceSpec,
  PluginErrorFallbackSpec,
  RpcAfterResponseHook,
  RpcBeforeRequestHook,
} from "../types";
import {
  AGENT_SOURCE,
  DATA_PROVIDER,
  ERROR_FALLBACK,
  RPC_AFTER_RESPONSE,
  RPC_BEFORE_REQUEST,
} from "../kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "./extensions";

/**
 * Pick the active agent source — highest priority wins, ties broken by
 * insertion order. Returns undefined if none registered.
 */
export function pickAgentSource(): AgentSourceSpec | undefined {
  const sources = lookupExtensionPoint(AGENT_SOURCE);
  if (sources.length === 0) return undefined;
  return sources.reduce((best, cur) => ((cur.priority ?? 0) > (best.priority ?? 0) ? cur : best));
}

/**
 * Look up the fetcher for a data-provider key. Type is erased — callers
 * cast to their expected return shape. Returns undefined when nothing
 * registered (consumer hooks should throw or fall back).
 */
export function lookupDataProvider<T = unknown, P = unknown>(
  key: string,
): ((params?: P) => Promise<T>) | undefined {
  const spec = lookupExtensionByKey(DATA_PROVIDER, key);
  return spec ? (spec.fetcher as (params?: P) => Promise<T>) : undefined;
}

/** Snapshot of registered beforeRequest hooks in insertion order. */
export function listRpcBeforeHooks(): RpcBeforeRequestHook[] {
  return lookupExtensionPoint(RPC_BEFORE_REQUEST);
}

/** Snapshot of registered afterResponse hooks in insertion order. */
export function listRpcAfterHooks(): RpcAfterResponseHook[] {
  return lookupExtensionPoint(RPC_AFTER_RESPONSE);
}

/**
 * Pick the highest-priority registered error fallback. Tied priorities
 * resolve by insertion order (later wins). Returns undefined when nothing
 * is registered.
 */
export function pickPluginErrorFallback(): PluginErrorFallbackSpec | undefined {
  const specs = lookupExtensionPoint(ERROR_FALLBACK);
  if (specs.length === 0) return undefined;
  return specs.reduce((best, cur) => ((cur.priority ?? 0) >= (best.priority ?? 0) ? cur : best));
}
