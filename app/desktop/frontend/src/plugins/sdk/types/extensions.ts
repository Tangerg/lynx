// Open extension points — the JetBrains-style "a plugin defines a typed
// point, any plugin contributes to it, any plugin consumes it" surface.
//
// Unlike layout slots (which carry React components rendered by <Slot>), an
// extension point carries arbitrary typed DATA / behaviour, consumed
// programmatically (a command list, a set of serializers, formatters…). The
// kernel owns none of these points — a plugin opens one with a string id and
// others fill it, exactly like `host.agui.on(name, …)` fans an event out to
// every registered handler.

/** How a point keys its contributions. */
export type ExtensionKeying =
  // Same key → overwrite the previous, warn on cross-plugin override.
  // Used by "there is one X per key" points (themes by id, previews by fn…).
  | "single"
  // Composite key per (plugin, id) → every contribution coexists.
  // Used by "many handlers / chips / slots" points (events, layout…).
  | "multi";

/**
 * A typed handle to an extension point. Created with `defineExtensionPoint`
 * and shared as a module const between the plugin that contributes and the
 * one that consumes — it re-adds the type inference the raw string API would
 * erase. The handle holds no state; the registry is the single source of
 * truth.
 */
export interface ExtensionPoint<T> {
  /** The string id this point is keyed by in the registry. */
  readonly id: string;
  /** Keying strategy — see `ExtensionKeying`. */
  readonly keying: ExtensionKeying;
  /**
   * How to derive the dedupe key from an item for `single` points. Defaults
   * to `item.id`. Use it for points keyed by something else (a tool fn name,
   * a data-provider `key`, a content-block `kind`).
   */
  readonly keyOf?: (item: T) => string;
  /**
   * Optional key normalizer for `single` points — e.g. shortcuts fold
   * "Cmd+K" / "mod+k" to one canonical combo. Applied to `keyOf`'s result on
   * both contribute and remove so registrations and lookups agree.
   */
  readonly normalizeKey?: (key: string) => string;
  /**
   * @internal Migration seam. Kernel points back onto an existing named
   * registry field while the 40-map → 1-map collapse is staged (L2→L3);
   * plugin-defined points omit this and live in the shared `extensions` map.
   */
  readonly field?: string;
  /** @internal phantom — carries `T` for inference; never read at runtime. */
  readonly __itemType?: T;
}

/** Per-contribution options passed to `host.extensions.contribute`. */
export interface ExtensionContributionOptions {
  /**
   * Stable id within a `multi` point — defaults to a minted one. Pass it so a
   * same-name plugin reload overwrites its prior contribution rather than
   * stacking a duplicate. Ignored by `single` points (they key via `keyOf`).
   */
  id?: string;
  /**
   * Explicit dedupe key for `single` points whose key isn't carried on the
   * item — a tool fn name, a content-block kind, a slash trigger. Takes
   * precedence over the point's `keyOf`. Ignored by `multi` points.
   */
  key?: string;
  /** Sort hint — lower comes first. Falls back to the item's own `order`. */
  order?: number;
}
