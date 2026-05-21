import { useEffect, useState } from "react";

// useDebouncedValue — settles `value` to the most recent input after it
// stays unchanged for `delayMs`. Used to gate expensive renderers (Shiki
// highlighter, Mermaid parser) so they don't re-run on every smooth-text
// delta when the source code/diagram is still streaming in.
//
// While the value is in flight (changing every few ms), the hook keeps
// returning the previous settled snapshot. Consumers can detect "still
// settling" with `value !== debounced`.
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [settled, setSettled] = useState(value);

  useEffect(() => {
    if (settled === value) return;
    const id = window.setTimeout(() => setSettled(value), delayMs);
    return () => window.clearTimeout(id);
  }, [value, delayMs, settled]);

  return settled;
}
