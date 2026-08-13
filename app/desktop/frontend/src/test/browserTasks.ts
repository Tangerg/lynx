/**
 * Drain browser-owned work after a component using a focus manager or motion
 * projection has unmounted. Callers must unmount first: real integration tests
 * own long-lived timers that a global drain would incorrectly consume.
 */
export function drainBrowserTasks(): Promise<void> {
  return (window as unknown as HappyDOMWindow).happyDOM.waitUntilComplete();
}
import type { Window as HappyDOMWindow } from "happy-dom";
