// The active appearance, as leaf code sees it.
//
// Two leaf modules need it: the motion presets scale every duration by the
// user's Motion preference, and the Shiki preset pairs with the active scheme.
// Both used to import `useUiStore` (and the THEME registry) directly from `lib`,
// which inverted the ring — and worse, laundered an edge the layer guard
// forbids: `ui/atoms/shiki-code-block` reached the global preference store and
// the plugin registry *through* `lib/highlight`, so the design system depended
// on both without ever importing them.
//
// So the dependency runs the other way. This holds a passive snapshot; the
// context that owns appearance (theme) publishes into it from the painter — the
// one place where a preference becomes visible. Nothing here reads a store, a
// registry, or the DOM.
//
// The defaults are what a snapshot-less app looks like (unscaled motion, dark):
// they apply only before the first publish, and in tests that don't install the
// painter.

import { useSyncExternalStore } from "react";

export type Scheme = "dark" | "light";

let scheme: Scheme = "dark";
let scale = 1;
const listeners = new Set<() => void>();

/** Publish the scheme the painter just applied. */
export function publishScheme(next: Scheme): void {
  if (next === scheme) return;
  scheme = next;
  for (const listener of listeners) listener();
}

/** Publish the motion multiplier the painter just applied. No notification —
 *  the presets read it when an animation starts, so there is nothing to
 *  re-render (a running animation keeps the scale it began with). */
export function publishMotionScale(next: number): void {
  scale = next;
}

export function motionScale(): number {
  return scale;
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

function snapshot(): Scheme {
  return scheme;
}

/** Reactive read — re-renders on a theme switch. */
export function useScheme(): Scheme {
  return useSyncExternalStore(subscribe, snapshot);
}
