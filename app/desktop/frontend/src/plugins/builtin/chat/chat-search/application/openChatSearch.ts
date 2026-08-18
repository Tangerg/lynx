// Opening the find bar is a capability of the find bar, not a message on a global
// event bus.
//
// One composer and one find bar exist per window, so a module-level handle is the
// whole capability. Callers in other contexts need not know how the overlay mounts.

let open: (() => void) | null = null;

/** Called by the overlay for as long as it is mounted. */
export function setChatSearchOpener(fn: (() => void) | null): void {
  open = fn;
}

export function openChatSearch(): void {
  open?.();
}
