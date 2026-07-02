import { useRef, useState } from "react";

// Transient state of an async "probe" — a save / test / delete whose result is
// shown inline (spinner → ok / error) on the control that launched it.
type Probe = { state: "idle" | "busy" } | { state: "ok" } | { state: "error"; reason: string };

/**
 * Drive an inline async-operation indicator with stale-result de-racing.
 *
 * A monotonic token guards every {@link run}: a result whose token is no longer
 * current — a newer run started, or `reset` bumped it — is dropped, so a slow
 * test cannot overwrite the state of a save the user kicked off afterwards.
 * `reset` invalidates any in-flight run and clears the readout; `fail` sets an
 * error directly (for flows, like delete, that don't need the de-race guard).
 */
export function useProbe() {
  const [probe, setProbe] = useState<Probe>({ state: "idle" });
  const seq = useRef(0);

  const reset = () => {
    seq.current++;
    setProbe({ state: "idle" });
  };

  const fail = (reason: string) => setProbe({ state: "error", reason });

  const run = async (op: () => Promise<{ ok: boolean; error?: string }>, fallback: string) => {
    const token = ++seq.current;
    setProbe({ state: "busy" });
    try {
      const r = await op();
      if (seq.current !== token) return;
      setProbe(r.ok ? { state: "ok" } : { state: "error", reason: r.error ?? fallback });
    } catch (err) {
      if (seq.current !== token) return;
      fail(err instanceof Error ? err.message : fallback);
    }
  };

  return { probe, reset, fail, run };
}
