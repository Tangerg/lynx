import { useSyncExternalStore } from "react";

// Whether the transcript is scrolled to its tail, published out of the scroll
// provider for the one consumer that lives outside it (the jump-to-bottom button,
// which must be a sibling of the scroller to sit over the composer).
//
// A passive snapshot rather than state on the surrounding component:
// `use-stick-to-bottom` rebuilds its context object on every scroll event, so a
// relay whose effect depends on it fires at scroll frequency. Publishing that object
// through the parent would re-render the entire chat surface, including transcript,
// banners, and composer, for ordinary scrolling.
//
// Only `atBottom` is reactive. `scrollToBottom` is called from a click handler, so
// it is read when it is needed and publishing a new one notifies nobody — which is
// the whole point: a scroll that doesn't cross the threshold re-renders nothing.

let atBottom = true;
let scrollToBottom = (): void => {};
const listeners = new Set<() => void>();

export function publishStreamFollow(next: { atBottom: boolean; scrollToBottom: () => void }): void {
  scrollToBottom = next.scrollToBottom;
  if (next.atBottom === atBottom) return;
  atBottom = next.atBottom;
  for (const listener of listeners) listener();
}

/** Scroll the transcript to its tail — the button's click, run imperatively. */
export function scrollStreamToBottom(): void {
  scrollToBottom();
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

/** Reactive read — re-renders only when the transcript crosses its tail. */
export function useStreamAtBottom(): boolean {
  return useSyncExternalStore(subscribe, () => atBottom);
}
