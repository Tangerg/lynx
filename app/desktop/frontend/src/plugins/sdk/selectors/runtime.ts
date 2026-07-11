// Runtime / data-layer selectors — routes, agent sources, data providers,
// and plugin error fallback. The grab-bag of "things plugins
// register that don't belong to a specific UI surface".

import type {
  AgentRunStartOptions,
  AgentRunOptionsProviderSpec,
  AgentSourceSpec,
  PluginErrorFallbackSpec,
} from "../types";
import { AGENT_RUN_OPTIONS, AGENT_SOURCE, DATA_PROVIDER, ERROR_FALLBACK } from "../kernelPoints";
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

function pickAgentRunOptionsProvider(): AgentRunOptionsProviderSpec | undefined {
  const providers = lookupExtensionPoint(AGENT_RUN_OPTIONS);
  if (providers.length === 0) return undefined;
  return providers.reduce((best, cur) =>
    (cur.priority ?? 0) >= (best.priority ?? 0) ? cur : best,
  );
}

export function resolveAgentRunStartOptions(): AgentRunStartOptions {
  return pickAgentRunOptionsProvider()?.resolve() ?? {};
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
