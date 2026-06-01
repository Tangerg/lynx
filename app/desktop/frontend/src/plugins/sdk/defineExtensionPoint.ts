// `defineExtensionPoint` — create a typed handle to an extension point.
//
// Share the returned const between the contributing plugin and the consuming
// one: it carries the string `id` plus the element type `T`, so
// `host.extensions.contribute(point, item)` type-checks `item` and
// `useExtensionPoint(point)` / `lookupExtensionPoint(point)` come back typed.
//
// Like `definePlugin`, this is an identity function — the value it returns is
// the descriptor itself. Kept as a function (not a bare object) so the door
// stays open for validation / branding without touching call sites.

import type { ExtensionKeying, ExtensionPoint } from "./types/extensions";

export function defineExtensionPoint<T>(def: {
  id: string;
  keying: ExtensionKeying;
  keyOf?: (item: T) => string;
  normalizeKey?: (key: string) => string;
  /** @internal kernel-only migration seam — see ExtensionPoint.field. */
  field?: string;
}): ExtensionPoint<T> {
  return def;
}
