// Read side of the open-extension-point substrate.
//
// Both surfaces resolve a point id to its contributed items, sorted by
// `order`. The secondary index (`byPoint`) caches on the source Map
// reference — the registry produces a fresh `extensions` Map on every
// contribute/remove, so the cache auto-invalidates on mutation and steady-
// state reads (during streaming, when nothing registers) stay O(1).

import type { ContributionEntry, Owned } from "../registryState";
import type { ExtensionPoint } from "../types/extensions";
import { useMemo } from "react";
import { usePluginStore } from "../registry";
import { createIndex } from "./_helpers";

interface Resolved {
  order?: number;
  item: unknown;
}

const byPoint = createIndex<ContributionEntry, Resolved>((o) => ({
  key: o.value.point,
  value: { order: o.value.order, item: o.value.item },
}));

// Sort hint precedence: the item's own `order` (the legacy spec field every
// kernel slot already carries) wins, then the contribute-time `opts.order`,
// then a stable default. Array#sort is stable so equal orders keep insertion
// order — matching the previous per-slot `useSortedList` behaviour.
function sortKey(e: Resolved): number {
  const own = (e.item as { order?: number } | null)?.order;
  return own ?? e.order ?? 100;
}

function resolve<T>(extensions: Map<string, Owned<ContributionEntry>>, pointId: string): T[] {
  const list = byPoint(extensions).get(pointId) ?? [];
  return [...list].sort((a, b) => sortKey(a) - sortKey(b)).map((e) => e.item) as T[];
}

/** Imperative read of every contribution to `point`, sorted by order. */
export function lookupExtensionPoint<T>(point: ExtensionPoint<T>): T[] {
  return resolve<T>(usePluginStore.getState().extensions, point.id);
}

/** React hook — re-renders when the point's contributions change. */
export function useExtensionPoint<T>(point: ExtensionPoint<T>): T[] {
  const extensions = usePluginStore((s) => s.extensions);
  return useMemo(() => resolve<T>(extensions, point.id), [extensions, point.id]);
}

/**
 * O(1) lookup of a single contribution by its dedupe key — for `single`
 * points where callers want "the X registered for this id/fn/combo" without
 * scanning the list (themes by id, tool icons by fn, commands by id…).
 * Applies the point's `normalizeKey` so lookups match how it was stored.
 */
export function lookupExtensionByKey<T>(point: ExtensionPoint<T>, key: string): T | undefined {
  const k = point.normalizeKey ? point.normalizeKey(key) : key;
  const entry = usePluginStore.getState().extensions.get(`${point.id}#${k}`);
  return entry?.value.item as T | undefined;
}

/**
 * Reactive sibling of `lookupExtensionByKey` — subscribes to exactly one
 * `single`-point slot so a component re-renders only when that key's
 * contribution changes (replaces the old `usePluginStore(s => s.X.get(id))`).
 */
export function useExtensionByKey<T>(point: ExtensionPoint<T>, key: string): T | undefined {
  const k = point.normalizeKey ? point.normalizeKey(key) : key;
  const outerKey = `${point.id}#${k}`;
  return usePluginStore((s) => s.extensions.get(outerKey)?.value.item as T | undefined);
}

/**
 * Owner plugin of a single contribution — for error attribution (which
 * plugin's tool action threw). Returns undefined when nothing is registered
 * under the key.
 */
export function lookupExtensionOwner<T>(point: ExtensionPoint<T>, key: string): string | undefined {
  const k = point.normalizeKey ? point.normalizeKey(key) : key;
  return usePluginStore.getState().extensions.get(`${point.id}#${k}`)?.pluginName;
}
