// Opening the find bar is a capability of the find bar, not a message on a global
// event bus.
//
// It used to be `window.dispatchEvent(new Event("lyra.chat-search.open"))` with
// the overlay listening — which meant the use case could not be exercised without
// a browser, and the app carried two different mechanisms for the same need
// (the composer's focus already worked this way). One composer, one find bar per
// window, so a module-level handle beats a bus: the caller is a keyboard shortcut
// in another context and has no business knowing how this overlay mounts.

let open: (() => void) | null = null;

/** Called by the overlay for as long as it is mounted. */
export function setChatSearchOpener(fn: (() => void) | null): void {
  open = fn;
}

export function openChatSearch(): void {
  open?.();
}
