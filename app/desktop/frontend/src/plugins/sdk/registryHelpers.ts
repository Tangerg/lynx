// Map mutation helpers shared by the registry's remaining bookkeeping maps.
// Pulled out of `registryState.ts` so that file is purely shape (types +
// `freshState`) and this file is purely behaviour. The Zustand store in
// `registry.ts` imports both.
//
// What's left after the extension-point collapse: the declared-* placeholder
// maps use {add,remove}Owned (caller-supplied key, conflict-warn on override)
// plus `clearByPlugin` for the activation flow; `ownedContributionsTo` reads
// the shared `extensions` substrate from inside the store's own lifecycle
// loops; `mapSet`/`mapDrop` cover the plain (loaded / pendingActivations)
// Maps.

import type { ContributionEntry, Owned } from "./registryState";

// Owned contributions to one point on the shared `extensions` map, in
// insertion order. Used by the store's own lifecycle-firing loops
// (markAppReady / registerLoaded / unload), which can't import the selectors
// (those import the registry). The owner is preserved for error attribution.
export function ownedContributionsTo(
  extensions: Map<string, Owned<ContributionEntry>>,
  pointId: string,
): Array<Owned<ContributionEntry>> {
  const out: Array<Owned<ContributionEntry>> = [];
  for (const o of extensions.values()) {
    if (o.value.point === pointId) out.push(o);
  }
  return out;
}

// Immutable single-key update of a plain (non-Owned) Map field — the
// `const next = new Map(prev); next.set/delete(...); return next` ritual
// Zustand actions repeat for every Map-valued slot. Owned slots use the
// {add,remove}Owned* helpers below; these two cover the registry's plain
// bookkeeping Maps (loaded plugins, pending activations).
export function mapSet<K, V>(map: Map<K, V>, key: K, value: V): Map<K, V> {
  const next = new Map(map);
  next.set(key, value);
  return next;
}

export function mapDrop<K, V>(map: Map<K, V>, key: K): Map<K, V> {
  const next = new Map(map);
  next.delete(key);
  return next;
}

export function addOwned<T>(
  map: Map<string, Owned<T>>,
  pluginName: string,
  key: string,
  value: T,
  label: string,
): Map<string, Owned<T>> {
  const existing = map.get(key);
  if (existing && existing.pluginName !== pluginName) {
    console.warn(
      `[plugin] ${pluginName} overrides ${label} "${key}" ` +
        `previously registered by ${existing.pluginName}`,
    );
  }
  const next = new Map(map);
  next.set(key, { pluginName, value });
  return next;
}

export function removeOwned<T>(
  map: Map<string, Owned<T>>,
  pluginName: string,
  key: string,
): Map<string, Owned<T>> {
  const entry = map.get(key);
  if (!entry || entry.pluginName !== pluginName) return map;
  const next = new Map(map);
  next.delete(key);
  return next;
}

// "Drop every entry a given plugin owns" — used by the declared-* slots
// when a plugin's activation replaces its entire batch of placeholder
// contributions at once.
export function clearByPlugin<T>(
  map: Map<string, Owned<T>>,
  pluginName: string,
): Map<string, Owned<T>> {
  const next = new Map(map);
  for (const [k, v] of next) if (v.pluginName === pluginName) next.delete(k);
  return next;
}
