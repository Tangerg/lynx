// Read side of the open-extension-point substrate — the one read API for both
// kernel and plugins. Four public reads, two axes (hook vs imperative · whole
// list vs one key):
//
//   useExtensionPoint(P)        hook   → T[]  (re-renders on change, sorted)
//   lookupExtensionPoint(P)     plain  → T[]  (reducer / setup / event-time)
//   useExtensionByKey(P, k)     hook   → T?   (subscribes to one slot only)
//   lookupExtensionByKey(P, k)  plain  → T?   (O(1) by dedupe key)
//
// `use*` for render, `lookup*` everywhere else; `*ByKey` when you want one entry
// by id/fn/combo, `*Point` for the whole list. `useExtensionEntries` is the rare
// "I also need each item's key" variant (slash triggers). `lookupExtensionOwner`
// / `lookupExtensionOwnedEntries` surface the contributing plugin for error
// attribution. `createPointSubIndex` is the engine for sub-keyed fan-out
// (events by type, layout by slot) — not a general read.
//
// All reads share `byPoint`, a secondary index cached on the `extensions` Map
// reference: the registry mints a fresh Map per contribute/remove, so the cache
// auto-invalidates on mutation and steady-state reads (streaming, no registers)
// stay O(1).

import type { ContributionEntry, Owned } from "../registryState";
import type { ExtensionPoint } from "../types/extensions";
import { useMemo } from "react";
import { usePluginStore } from "../registry";
import { ownedContributionsTo } from "../registryMutations";

// The one structural invariant shared by the write path (host.contribute) and
// every read: a contribution lives under `${point.id}#${dedupe}` in the flat
// `extensions` map. `dedupe` is the normalized single key, or `${plugin}|${id}`
// for multi points. Keep the format here so write + read can't drift.
export function composeExtensionKey(pointId: string, dedupe: string): string {
  return `${pointId}#${dedupe}`;
}

/** Map key for a single-point lookup — applies the point's `normalizeKey`. */
function slotKeyOf<T>(point: ExtensionPoint<T>, key: string): string {
  return composeExtensionKey(point.id, point.normalizeKey ? point.normalizeKey(key) : key);
}

interface FlatContribution {
  key: string;
  order?: number;
  item: unknown;
}

// One cached secondary-index engine shared by `byPoint` (bucket every
// contribution by its point id) and `createPointSubIndex` (bucket one point's
// contributions by a derived sub-key). Caches on the source Map reference — the
// registry mints a fresh Map per contribute/remove, so the cache auto-
// invalidates on mutation and steady-state reads (streaming, no registers) stay
// O(1). `extract` returns null to drop an entry from the index.
function cachedBucketIndex<V>(
  extract: (owned: Owned<ContributionEntry>) => { key: string; value: V } | null,
) {
  const cache = new WeakMap<Map<string, Owned<ContributionEntry>>, Map<string, V[]>>();
  return (source: Map<string, Owned<ContributionEntry>>): Map<string, V[]> => {
    const cached = cache.get(source);
    if (cached) return cached;
    const idx = new Map<string, V[]>();
    for (const o of source.values()) {
      const entry = extract(o);
      if (!entry) continue;
      const list = idx.get(entry.key);
      if (list) list.push(entry.value);
      else idx.set(entry.key, [entry.value]);
    }
    cache.set(source, idx);
    return idx;
  };
}

const byPoint = cachedBucketIndex<FlatContribution>((o) => ({
  key: o.value.point,
  value: { key: o.value.key, order: o.value.order, item: o.value.item },
}));

// Sort hint precedence: the item's own `order` field wins, then the
// contribute-time `opts.order`, then a stable default. Array#sort is
// stable so equal orders keep insertion order.
function sortKey(e: FlatContribution): number {
  const own = (e.item as { order?: number } | null)?.order;
  return own ?? e.order ?? 100;
}

/** Contribution paired with its dedupe key, for points keyed by a value the
 * item doesn't carry (slash trigger, tool fn). */
export interface ExtensionEntry<T> {
  key: string;
  item: T;
}

function resolveEntries(
  extensions: Map<string, Owned<ContributionEntry>>,
  pointId: string,
): FlatContribution[] {
  const list = byPoint(extensions).get(pointId) ?? [];
  return [...list].sort((a, b) => sortKey(a) - sortKey(b));
}

function resolve<T>(extensions: Map<string, Owned<ContributionEntry>>, pointId: string): T[] {
  return resolveEntries(extensions, pointId).map((e) => e.item) as T[];
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

/** Hook variant of `useExtensionPoint` that keeps each item's dedupe key. */
export function useExtensionEntries<T>(point: ExtensionPoint<T>): Array<ExtensionEntry<T>> {
  const extensions = usePluginStore((s) => s.extensions);
  return useMemo(
    () => resolveEntries(extensions, point.id).map((e) => ({ key: e.key, item: e.item as T })),
    [extensions, point.id],
  );
}

/**
 * O(1) lookup of a single contribution by its dedupe key — for `single`
 * points where callers want "the X registered for this id/fn/combo" without
 * scanning the list (themes by id, tool icons by fn, commands by id…).
 * Applies the point's `normalizeKey` so lookups match how it was stored.
 */
export function lookupExtensionByKey<T>(point: ExtensionPoint<T>, key: string): T | undefined {
  const entry = usePluginStore.getState().extensions.get(slotKeyOf(point, key));
  return entry?.value.item as T | undefined;
}

/**
 * Reactive sibling of `lookupExtensionByKey` — subscribes to exactly one
 * `single`-point slot so a component re-renders only when that key's
 * contribution changes (replaces the old `usePluginStore(s => s.X.get(id))`).
 */
export function useExtensionByKey<T>(point: ExtensionPoint<T>, key: string): T | undefined {
  const outerKey = slotKeyOf(point, key);
  return usePluginStore((s) => s.extensions.get(outerKey)?.value.item as T | undefined);
}

/**
 * Owner plugin of a single contribution — for error attribution (which
 * plugin's tool action threw). Returns undefined when nothing is registered
 * under the key.
 */
export function lookupExtensionOwner<T>(point: ExtensionPoint<T>, key: string): string | undefined {
  return usePluginStore.getState().extensions.get(slotKeyOf(point, key))?.pluginName;
}

/**
 * Cached secondary index over one point's contributions, bucketed by a sub-key
 * derived from each item (event type, slot name…). The reducer hits this per
 * StreamEvent; insertion order within a bucket is preserved. Caching contract is
 * `cachedBucketIndex`'s.
 */
export function createPointSubIndex<I, V>(
  pointId: string,
  extract: (item: I, pluginName: string) => { key: string; value: V },
) {
  return cachedBucketIndex<V>((o) =>
    o.value.point === pointId ? extract(o.value.item as I, o.pluginName) : null,
  );
}

/** Item paired with its owner plugin. */
export interface ExtensionOwnedEntry<T> {
  pluginName: string;
  item: T;
}

/**
 * Every contribution to a `multi` point with its owner — for fan-out loops that
 * attribute per-handler errors back to the contributing plugin (the global
 * beforeunload listener). Insertion order; not a hook (imperative call sites).
 */
export function lookupExtensionOwnedEntries<T>(
  point: ExtensionPoint<T>,
): Array<ExtensionOwnedEntry<T>> {
  return ownedContributionsTo(usePluginStore.getState().extensions, point.id).map((o) => ({
    pluginName: o.pluginName,
    item: o.value.item as T,
  }));
}
