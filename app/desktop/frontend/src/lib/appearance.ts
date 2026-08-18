// The active appearance, as leaf code sees it.
//
// Leaf modules use it for scheme-aware rendering and style-aware motion without
// reaching into the preference store or plugin registry, which would invert the
// dependency ring from leaf UI into application composition.
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

/**
 * How much of the accent's hue the neutral surfaces carry.
 *
 * The enum lives here rather than beside the maths that consumes it (see
 * `theme/kit/accentTint`) for the same reason every other appearance value does: the
 * preference store persists it, and `state` sits below the plugins.
 *
 * A preference rather than a constant because it is a taste axis, and the systems that
 * have solved this settle taste axes the same way — Material ships it as a scheme
 * variant (neutral chroma 6 / 10 / 2) rather than picking one and defending it.
 */
export const ACCENT_TINTS = ["off", "soft", "standard"] as const;
export type AccentTint = (typeof ACCENT_TINTS)[number];

/** What every surface in this app was measured against, so the default changes nothing. */
export const DEFAULT_ACCENT_TINT: AccentTint = "standard";

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
  /** Evenly spaced samples of the structural-panel spring, published as CSS `linear()`. */
  drawerProgress: readonly [number, number, ...number[]];
  pressScale: number;
}

/**
 * What motion is before a visual style publishes its own — the first frames, and any
 * consumer standing without the theme pack installed.
 *
 * Every value here has to match the shipped style (WORKBENCH_MOTION); a fallback that
 * disagrees is a fallback nobody notices is wrong. `drawerMs` had drifted to 300 against
 * the style's 240 that way.
 */
const DEFAULT_MOTION: VisualStyleMotion = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  disclosureMs: 220,
  slowMs: 360,
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
  easeInOut: [0.45, 0, 0.55, 1],
  easeEmphasized: [0.16, 1, 0.3, 1],
  drawerProgress: [
    0, 0.06981, 0.21761, 0.38345, 0.53716, 0.66615, 0.76765, 0.84375, 0.89859, 0.93672, 0.96233,
    0.97894, 0.98929, 0.99544, 0.99887, 1.00061, 1.00135, 1.00152, 1.00142, 1.00119, 1,
  ],
  pressScale: 0.98,
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
 * to re-read, and only the painter knows that. Consumers subscribe to appearance
 * replacement rather than guessing which preferences affect computed colours.
 */
export function publishTokens(): void {
  tokenRevision += 1;
  announce();
}

/** Publish the motion multiplier the painter just applied. */
export function publishMotionScale(next: number): void {
  if (scale === next) return;
  scale = next;
  announce();
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
