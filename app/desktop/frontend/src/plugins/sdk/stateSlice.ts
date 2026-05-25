// Cross-plugin state slices — a named, shared zustand store.
//
// Use case: plugin A produces some data (e.g. "currently-focused file");
// plugin B (a workspace view) wants to read + display it without forming
// a hard module-level dependency. They both call
// `host.state.slice("focused-file", "")` and operate on the same store.
//
// Sharing happens by name. The first caller's `initial` wins; later
// callers receive the existing slice (their `initial` is ignored).
//
// Slices live until the process exits (no per-plugin unload — they can
// be useful across plugin restarts) but the registry is reset in tests.

import type { Disposable } from "./types";
import { useSyncExternalStore } from "react";

export interface StateSlice<T> {
  /** Current value. */
  get: () => T;
  /** Set or update the value; listeners notified when it changes by Object.is. */
  set: (next: T | ((prev: T) => T)) => void;
  /** Subscribe to changes; returns a Disposable. */
  subscribe: (listener: (state: T) => void) => Disposable;
  /** React hook — selector pattern, identical to a zustand store. */
  useStore: <U = T>(selector?: (state: T) => U) => U;
}

// Implementation. Kept hand-written (not `create()` from zustand) because:
//   1. Each slice is a new instance; using zustand here would mean shipping
//      one create() per slice, which is fine but the manual version is
//      simpler and lets us reuse a single Map of slices.
//   2. We need React 18 useSyncExternalStore semantics, which is what
//      the hand-rolled implementation gives us directly.

const slices = new Map<string, StateSlice<unknown>>();

function makeSlice<T>(initial: T): StateSlice<T> {
  let value: T = initial;
  const listeners = new Set<(state: T) => void>();

  const get = () => value;
  const set = (next: T | ((prev: T) => T)) => {
    const computed = typeof next === "function" ? (next as (prev: T) => T)(value) : next;
    if (Object.is(computed, value)) return;
    value = computed;
    // Snapshot for safe iteration.
    for (const l of [...listeners]) l(value);
  };
  const subscribe = (listener: (state: T) => void): Disposable => {
    listeners.add(listener);
    return { dispose: () => listeners.delete(listener) };
  };
  const useStore = <U = T>(selector?: (state: T) => U): U => {
    const sel = selector ?? ((s: T) => s as unknown as U);
    return useSyncExternalStore(
      (onChange) => {
        const d = subscribe(() => onChange());
        return () => d.dispose();
      },
      () => sel(value),
      // SSR snapshot — same as client for us; no server rendering.
      () => sel(value),
    );
  };
  return { get, set, subscribe, useStore };
}

/**
 * Return (or create) the slice for `name`. Multiple calls with the same
 * name return the same slice; `initial` is honoured only by the first call.
 */
export function getOrCreateSlice<T>(name: string, initial: T): StateSlice<T> {
  const existing = slices.get(name);
  if (existing) return existing as StateSlice<T>;
  const slice = makeSlice<T>(initial);
  slices.set(name, slice as StateSlice<unknown>);
  return slice;
}

/** Test-only: wipe all slices so specs don't carry over. */
export function _resetAllSlices(): void {
  slices.clear();
}
