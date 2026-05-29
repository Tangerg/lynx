// Map mutation helpers shared by every registry slot. Pulled out of
// `registryState.ts` so that file is purely shape (types + `freshState`)
// and this file is purely behaviour. The Zustand store in `registry.ts`
// imports both.
//
// Every registry slot is a `Map<string, Owned<T>>`. We expose two key
// strategies:
//   - "single" via {add,remove}Owned — caller-supplied key, conflict-warn
//     when another plugin overrides
//   - "multi"  via {add,remove}OwnedMulti — auto composite key
//     `${pluginName}|${id}` so multiple registrations per plugin coexist
// plus a bulk `clearByPlugin` for the activation flow.

import type { Owned } from "./registryState";

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

// ---- composite-key helpers (multi-registration slots) -----------------
//
// For slots that allow multiple registrations per (plugin, id) — RPC hooks,
// log subscribers, lifecycle hooks, plugin observers, core event handlers,
// layout slots — the key is `${pluginName}|${id}` (or a discriminated id
// like `${slot}#${spec.id}` baked in by the caller). No conflict warning
// is meaningful for these: multiple entries per plugin are intentional.

function compositeKey(pluginName: string, id: string): string {
  return `${pluginName}|${id}`;
}

export function addOwnedMulti<T>(
  map: Map<string, Owned<T>>,
  pluginName: string,
  id: string,
  value: T,
): Map<string, Owned<T>> {
  const next = new Map(map);
  next.set(compositeKey(pluginName, id), { pluginName, value });
  return next;
}

export function removeOwnedMulti<T>(
  map: Map<string, Owned<T>>,
  pluginName: string,
  id: string,
): Map<string, Owned<T>> {
  const next = new Map(map);
  next.delete(compositeKey(pluginName, id));
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
