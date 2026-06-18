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

import type { ExtensionPoint } from "./types/extensions";

// The descriptor a caller passes — the whole `ExtensionPoint` shape minus the
// phantom `__itemType` (carried for inference, never written).
export function defineExtensionPoint<T>(
  def: Omit<ExtensionPoint<T>, "__itemType">,
): ExtensionPoint<T> {
  return def;
}
