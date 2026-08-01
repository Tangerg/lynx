// The active appearance, as leaf code sees it.
//
// Leaf modules use it for scheme-aware rendering and style-aware motion without
// reaching into the preference store or plugin registry. Those reads used to
// import `useUiStore` (and the colour-theme registry) directly from `lib`,
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

/**
 * Light or dark, the app's whole vocabulary for it.
 *
 * Declared here because this is the one module every ring may import — the
 * design system, the plugin SDK's theme contract, the theme context's rules and
 * kit, and the panes that preview a swatch all need the same two words. Eight
 * modules had spelled the union out inline instead, which is eight places that
 * must agree forever with nothing checking that they do.
 */
export type Scheme = "dark" | "light";
export type ColorThemeId = string;
export type VisualStyleId = string;

export interface VisualStyleMotion {
  instantMs: number;
  fastMs: number;
  mediumMs: number;
  disclosureMs: number;
  slowMs: number;
  drawerMs: number;
  easeOut: readonly [number, number, number, number];
  easeInOut: readonly [number, number, number, number];
  easeEmphasized: readonly [number, number, number, number];
  easeDrawer: readonly [number, number, number, number];
  pressScale: number;
}

const DEFAULT_MOTION: VisualStyleMotion = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  disclosureMs: 220,
  slowMs: 360,
  drawerMs: 300,
  easeOut: [0.22, 1, 0.36, 1],
  easeInOut: [0.45, 0, 0.55, 1],
  easeEmphasized: [0.16, 1, 0.3, 1],
  easeDrawer: [0.32, 0.72, 0, 1],
  pressScale: 0.96,
};

let scheme: Scheme = "dark";
let scale = 1;
let motion = DEFAULT_MOTION;
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

/** Publish the active style's motion language for non-CSS animation consumers. */
export function publishVisualStyleMotion(next: VisualStyleMotion): void {
  motion = next;
}

export function motionScale(): number {
  return scale;
}

export function visualStyleMotion(): VisualStyleMotion {
  return motion;
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
