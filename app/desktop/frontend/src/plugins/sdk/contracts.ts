// Extension points, as Contract tokens.
//
// A point is a dougong `ExtensionPoint` token plus the policy the old kernel used
// to enforce around it: how a contribution's domain key is derived, and how
// that key is normalized so a registration and a lookup of the same combo agree.
//
// The token carries a `Contribution<T>` envelope rather than the bare item,
// because Core keys a contribution by its OWNER — `plugin:installation/key` — a
// key that changes when a plugin is reinstalled and that no consumer can
// construct. So the domain key travels inside the value. `order` travels with it
// for a second reason: Core's view is an unordered map on purpose, and sort
// position is a property of the contribution, not of the store.
//
// `keying` no longer changes how anything is stored — every contribution
// coexists under its own owner-qualified key. It survives as a READ policy:
// `single` means the last contributor of a domain key wins. That is a real
// behaviour change and an improvement — when the overriding plugin unloads, the
// contribution it shadowed comes back, where before it had been destroyed.

import { extensionPoint } from "dougong";
import type { ExtensionKeying, ExtensionPoint } from "./types/extensions";

/**
 * The envelope every contribution is stored in.
 *
 * `key` is the domain key (theme id, tool fn, normalized combo) — the thing
 * `lookupExtensionByKey` matches on. `plugin` is the owner, kept for error
 * attribution. `order` is the contribute-time sort hint; a value that carries its
 * own `order` field outranks it.
 */
export interface Contribution<T> {
  readonly key: string;
  readonly order: number | undefined;
  readonly plugin: string;
  readonly item: T;
}

// Ids already taken. Two points under one id would silently share a store, and
// the second definition would read the first's contributions as its own.
const taken = new Set<string>();

interface ExtensionPointSpec<T> {
  readonly id: string;
  readonly keying: ExtensionKeying;
  readonly keyOf?: (item: T) => string;
  readonly normalizeKey?: (key: string) => string;
}

/**
 * Create a typed handle to an extension point.
 *
 * Share the returned const between the contributing plugin and the consuming
 * one: it carries the element type `T` plus the Contract token the kernel routes
 * on, so `contribute(point, item)` type-checks `item` and the read selectors come
 * back typed.
 */
export function defineExtensionPoint<T>(spec: ExtensionPointSpec<T>): ExtensionPoint<T> {
  if (taken.has(spec.id)) {
    throw new Error(`Extension point "${spec.id}" is already defined`);
  }
  const point: ExtensionPoint<T> = {
    ...spec,
    token: extensionPoint<Contribution<T>>(spec.id),
  };
  taken.add(spec.id);
  return point;
}
