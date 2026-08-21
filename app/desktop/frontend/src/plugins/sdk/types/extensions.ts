// Open extension points — the "a plugin defines a typed point, any plugin
// contributes to it, any plugin consumes it" surface.
//
// Unlike layout slots (which carry React components rendered by <Slot>), an
// extension point carries arbitrary typed DATA / behaviour, consumed
// programmatically (a command list, a set of serializers, formatters…). The
// kernel owns none of these points — a plugin opens one with a string id and
// others fill it.
//
// The handle is created by `defineExtensionPoint` (see `../contracts`), which is
// where the storage envelope and the reason it exists are documented.

import type { ExtensionPoint as ContractToken } from "dougong";
import type { Contribution } from "../contracts";

/** How a point resolves two contributions that claim the same domain key. */
export type ExtensionKeying =
  // Last contributor of a key wins; the ones it shadows stay in the store and
  // come back if it unloads. "There is one X per key" points — themes by id,
  // previews by tool fn, commands by id.
  | "single"
  // Every contribution stands on its own; nothing shadows anything. "Many
  // handlers / chips / slots" points — events, layout, log subscribers.
  | "multi";

/**
 * A typed handle to an extension point, shared as a module const between the
 * plugin that contributes and the one that consumes — it re-adds the type
 * inference a raw string API would erase. The handle holds no state; the Host's
 * contribution store is the single source of truth.
 */
export interface ExtensionPoint<T> {
  /** The string id this point is known by, and its Contract token's id. */
  readonly id: string;
  /** How the read side resolves a contested domain key — see `ExtensionKeying`. */
  readonly keying: ExtensionKeying;
  /** The Contract the Host routes contributions on. */
  readonly token: ContractToken<Contribution<T>>;
  /**
   * How to derive the domain key from an item for `single` points. Defaults to
   * `item.id`. Use it for points keyed by something else (a tool fn name, a
   * data-provider `key`, a content-block `kind`).
   */
  readonly keyOf?: (item: T) => string;
  /**
   * Optional key normalizer — e.g. shortcuts fold "Cmd+K" / "mod+k" to one
   * canonical combo. Applied on both contribute and lookup so registrations and
   * lookups agree.
   */
  readonly normalizeKey?: (key: string) => string;
}

/** Per-contribution options passed to `contribute`. */
export interface ExtensionContributionOptions {
  /**
   * Stable id within a `multi` point — defaults to a minted one. Pass it so a
   * same-name plugin reload overwrites its prior contribution rather than
   * stacking a duplicate. Ignored by `single` points (they key via `keyOf`).
   */
  id?: string;
  /**
   * Explicit domain key for `single` points whose key isn't carried on the item
   * — a tool fn name, a content-block kind, a slash trigger. Takes precedence
   * over the point's `keyOf`. Ignored by `multi` points.
   */
  key?: string;
  /** Sort hint — lower comes first. Falls back to the item's own `order`. */
  order?: number;
}
