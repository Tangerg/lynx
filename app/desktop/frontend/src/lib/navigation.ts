// Where the user is, as one value.
//
// The four scalars below are the whole answer to "what am I looking at": which
// session, which surface fills the content card, which dock destination is
// beside it, which settings pane is open. They live in router search params so
// browser history contains the complete location; this is the app's read/write contract.
//
// THE OWNERSHIP RULE, because getting it wrong is how a URL becomes a mirror:
// the location owns where you ARE; stores own what you KEPT (which dock tabs are
// open, which tools are expanded, which session to reopen on a cold start). A
// transition may READ memory to seed the location it navigates to. Nothing
// writes memory back into the location behind the user's back, and nothing keeps
// a second copy of these four.
//
// A port rather than a direct router import so the implementation is
// substitutable: the app installs a router-backed navigator, while visual
// fixtures and tests install an in-memory one instead of standing up a router
// they have no other use for.

import { createSingletonPort } from "./ports/singletonPort";

export interface AppLocation {
  /** Active session id; "" when none is selected. */
  session: string;
  /** A workspace view promoted to the whole content card; null is the chat. */
  view: string | null;
  /** The dock destination beside the chat; null means the dock is collapsed. */
  dock: string | null;
  /** The open settings pane; null when settings are closed. */
  settings: string | null;
}

export const EMPTY_LOCATION: AppLocation = {
  session: "",
  view: null,
  dock: null,
  settings: null,
};

export type LocationPatch = Partial<AppLocation>;

export interface Navigator {
  get(): AppLocation;
  /**
   * Reactive read. Select ONE field: the selected value is compared by identity,
   * so returning the whole location re-renders on every navigation.
   */
  use<T>(select: (location: AppLocation) => T): T;
  subscribe(listener: (location: AppLocation, previous: AppLocation) => void): () => void;
  /**
   * Move. Fields left out of the patch keep their current value; passing `null`
   * clears one. `replace` overwrites the current history entry instead of
   * pushing — for corrections that were never a place the user went, like
   * seeding the last session on a cold start.
   *
   * Do NOT wrap a call to this in `startTransition` expecting the render it causes to
   * be deferred. Switching session re-renders the whole transcript — measured at
   * 12–34ms of script per turn, so a few hundred ms on a long one — and a transition is
   * the obvious way to keep the click responsive. It does not work here, and the reason
   * is not obvious enough to rediscover: React de-opts a transition to a SYNCHRONOUS
   * render when the update arrives through `useSyncExternalStore`, which is how both
   * halves of this reach a component — the location through `use()` above, the transcript
   * through Zustand. Measured in a real browser on React 19.2: with `useState` as the
   * source the first frame after the click still carries the old content and `isPending`
   * is true; with an external store that frame already carries the new content, so
   * nothing was deferred and the second render a transition costs is spent for nothing.
   */
  go(patch: LocationPatch, options?: { replace?: boolean }): void;
  back(): void;
  forward(): void;
}

const port = createSingletonPort<Navigator>("Navigator port is not configured");

export const configureNavigator = port.configure;
export const navigator = port.get;

export function applyPatch(location: AppLocation, patch: LocationPatch): AppLocation {
  return { ...location, ...patch };
}

export function sameLocation(a: AppLocation, b: AppLocation): boolean {
  return (
    a.session === b.session && a.view === b.view && a.dock === b.dock && a.settings === b.settings
  );
}
