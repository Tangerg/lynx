// An in-memory Navigator, for the two harnesses that need a location but not a
// router: unit tests, and the visual fixtures (which render production
// components against frozen state and would otherwise have to stand up routing
// to photograph a dock tab).
//
// It keeps a real history stack, so a test can assert that going back returns
// to where it came from — the behaviour the router-backed one exists to provide.

import { useSyncExternalStore } from "react";
import {
  applyPatch,
  EMPTY_LOCATION,
  sameLocation,
  type AppLocation,
  type LocationPatch,
  type Navigator,
} from "./navigation";

export interface MemoryNavigator extends Navigator {
  /** Replace the whole location without recording history — fixture setup. */
  reset(location?: Partial<AppLocation>): void;
  entries(): AppLocation[];
}

export function createMemoryNavigator(initial: Partial<AppLocation> = {}): MemoryNavigator {
  let history: AppLocation[] = [applyPatch(EMPTY_LOCATION, initial)];
  let index = 0;
  const listeners = new Set<(location: AppLocation, previous: AppLocation) => void>();

  const current = (): AppLocation => history[index]!;

  const commit = (next: AppLocation, replace: boolean): void => {
    const previous = current();
    if (sameLocation(previous, next)) return;
    if (replace) history[index] = next;
    else {
      history = [...history.slice(0, index + 1), next];
      index = history.length - 1;
    }
    for (const listener of listeners) listener(next, previous);
  };

  const step = (delta: number): void => {
    const target = index + delta;
    if (target < 0 || target >= history.length) return;
    const previous = current();
    index = target;
    for (const listener of listeners) listener(current(), previous);
  };

  const subscribe = (
    listener: (location: AppLocation, previous: AppLocation) => void,
  ): (() => void) => {
    listeners.add(listener);
    return () => listeners.delete(listener);
  };

  return {
    get: current,
    // Reactive like the router-backed one. A fixture's location is frozen for
    // the shot and never exercises this, but a test that renders a component and
    // then navigates would otherwise silently observe the old value.
    use: (select) =>
      useSyncExternalStore(
        (onChange) => subscribe(() => onChange()),
        () => select(current()),
      ),
    subscribe,
    go(patch: LocationPatch, options) {
      commit(applyPatch(current(), patch), options?.replace === true);
    },
    back: () => step(-1),
    forward: () => step(1),
    reset(location = {}) {
      history = [applyPatch(EMPTY_LOCATION, location)];
      index = 0;
    },
    entries: () => [...history],
  };
}
