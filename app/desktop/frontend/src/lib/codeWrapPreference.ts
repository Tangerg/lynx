import { useSyncExternalStore } from "react";

type Listener = () => void;

let wrapCode = false;
const listeners = new Set<Listener>();

function subscribe(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function snapshot(): boolean {
  return wrapCode;
}

/** One application-lifetime reading preference, matching Codex's
 *  markdownCodeBlockWordWrap signal. It is deliberately not durable settings:
 *  a new Desktop process starts from unwrapped code again. */
export function useCodeWrapPreference(): boolean {
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}

export function setCodeWrapPreference(next: boolean): void {
  if (wrapCode === next) return;
  wrapCode = next;
  for (const listener of listeners) listener();
}

export function toggleCodeWrapPreference(): void {
  setCodeWrapPreference(!wrapCode);
}
