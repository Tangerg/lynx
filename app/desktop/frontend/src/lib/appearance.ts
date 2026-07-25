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
let tokenRevision = 0;
const listeners = new Set<() => void>();

function announce(): void {
  for (const listener of listeners) listener();
}

/** Publish the scheme the painter just applied. */
export function publishScheme(next: Scheme): void {
  if (next === scheme) return;
  scheme = next;
  announce();
}

/**
 * Publish that the colour tokens on `:root` were just rewritten.
 *
 * For the code that can't use a token — an SVG generator handed literal colours,
 * a canvas — and has to read the computed values instead. It needs to know WHEN
 * to re-read, and only the painter knows that. The alternative is what the mermaid
 * block used to do: subscribe to the two preferences it guessed were relevant
 * (theme, accent) and silently keep stale colours when a third one (contrast,
 * which moves the whole surface ladder) changed.
 */
export function publishTokens(): void {
  tokenRevision += 1;
  announce();
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

/** An opaque, monotonic stamp of the last token repaint — an invalidation key for
 *  anything that reads computed token values. */
export function useTokenRevision(): number {
  return useSyncExternalStore(subscribe, () => tokenRevision);
}
