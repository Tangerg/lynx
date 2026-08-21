// Read side of the extension-point substrate — the one read API for both kernel
// and plugins. Four public reads, two axes (hook vs imperative · whole list vs
// one key):
//
//   useExtensionPoint(P)        hook   → T[]  (re-renders on change, sorted)
//   lookupExtensionPoint(P)     plain  → T[]  (reducer / setup / event-time)
//   useExtensionByKey(P, k)     hook   → T?
//   lookupExtensionByKey(P, k)  plain  → T?   (by domain key)
//
// `use*` for render, `lookup*` everywhere else; `*ByKey` when you want one entry
// by id/fn/combo, `*Point` for the whole list. `useExtensionEntries` is the rare
// "I also need each item's key" variant (slash triggers). `lookupExtensionOwner`
// surfaces the contributing plugin for error attribution. `createPointSubIndex`
// is the engine for sub-keyed fan-out (events
// by type, layout by slot) — not a general read.
//
// Every derived structure caches on the entries array the catalog returns, which
// is stable by reference until that point's contributions change. So the cache
// invalidates exactly when the data does, and steady-state reads (streaming, no
// plugin churn) stay O(1) — the same property the old registry got from minting
// a fresh Map per mutation.

import { useMemo } from "react";
import type { Contribution } from "../contracts";
import { contributionsTo, useContributions } from "../kernel";
import type { ExtensionPoint } from "../types/extensions";

type Entries<T> = ReadonlyArray<Contribution<T>>;

/** Cache a derived structure on the identity of one point's entries array. */
function derived<T, V>(build: (entries: Entries<T>) => V): (entries: Entries<T>) => V {
  const cache = new WeakMap<object, V>();
  return (entries) => {
    const cached = cache.get(entries);
    if (cached !== undefined) return cached;
    const value = build(entries);
    cache.set(entries, value);
    return value;
  };
}

const itemsOf = derived(<T>(entries: Entries<T>) => entries.map((e) => e.item));

const byKey = derived(<T>(entries: Entries<T>) => new Map(entries.map((e) => [e.key, e] as const)));

function normalized<T>(point: ExtensionPoint<T>, key: string): string {
  return point.normalizeKey ? point.normalizeKey(key) : key;
}

/** Contribution paired with its domain key, for points keyed by a value the item
 *  doesn't carry (slash trigger, tool fn). */
export interface ExtensionEntry<T> {
  key: string;
  item: T;
}

/** Imperative read of every contribution to `point`, sorted by order. */
export function lookupExtensionPoint<T>(point: ExtensionPoint<T>): T[] {
  return itemsOf(contributionsTo(point)) as T[];
}

/** React hook — re-renders when the point's contributions change. */
export function useExtensionPoint<T>(point: ExtensionPoint<T>): T[] {
  const entries = useContributions(point);
  return useMemo(() => itemsOf(entries) as T[], [entries]);
}

/** Hook variant of `useExtensionPoint` that keeps each item's domain key. */
export function useExtensionEntries<T>(point: ExtensionPoint<T>): Array<ExtensionEntry<T>> {
  const entries = useContributions(point);
  return useMemo(() => entries.map((e) => ({ key: e.key, item: e.item })), [entries]);
}

/**
 * One contribution by its domain key — for `single` points where callers want
 * "the X registered for this id/fn/combo" without scanning (themes by id, tool
 * icons by fn, commands by id). Applies the point's `normalizeKey` so a lookup
 * matches how the contribution was stored.
 */
export function lookupExtensionByKey<T>(point: ExtensionPoint<T>, key: string): T | undefined {
  return byKey(contributionsTo(point)).get(normalized(point, key))?.item;
}

/** Reactive sibling of `lookupExtensionByKey`. */
export function useExtensionByKey<T>(point: ExtensionPoint<T>, key: string): T | undefined {
  const entries = useContributions(point);
  const wanted = normalized(point, key);
  return useMemo(() => byKey(entries).get(wanted)?.item, [entries, wanted]);
}

/**
 * Owner plugin of one contribution — for error attribution (which plugin's tool
 * action threw). Undefined when nothing is registered under the key.
 */
export function lookupExtensionOwner<T>(point: ExtensionPoint<T>, key: string): string | undefined {
  return byKey(contributionsTo(point)).get(normalized(point, key))?.plugin;
}

/**
 * Cached secondary index over one point's contributions, bucketed by a sub-key
 * derived from each item (event type, slot name…). The reducer hits this per
 * StreamEvent; insertion order within a bucket is preserved.
 *
 * Takes the point's entries so a hook can depend on the same array it renders
 * from; the map is recomputed only when that array is new.
 */
export function createPointSubIndex<I, V>(
  extract: (item: I, pluginName: string) => { key: string; value: V },
): (entries: Entries<I>) => ReadonlyMap<string, V[]> {
  return derived((entries: Entries<I>) => {
    const index = new Map<string, V[]>();
    for (const entry of entries) {
      const { key, value } = extract(entry.item, entry.plugin);
      const bucket = index.get(key);
      if (bucket) bucket.push(value);
      else index.set(key, [value]);
    }
    return index;
  });
}
