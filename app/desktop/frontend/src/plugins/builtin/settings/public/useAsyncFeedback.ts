import { useLayoutEffect, useRef, useState } from "react";

export type AsyncFeedback =
  { state: "idle" | "busy" } | { state: "ok" } | { state: "error"; reason: string };

/**
 * Drive an inline async-operation indicator with stale-result de-racing.
 *
 * A monotonic token guards every {@link run}: a result whose token is no longer
 * current — a newer run started, or `reset` bumped it — is dropped, so a slow
 * operation cannot overwrite feedback for a newer intent or replacement resource.
 * An optional material generation retires both completed feedback and in-flight
 * results without remounting or discarding the caller's draft fields.
 * `reset` invalidates any in-flight run and clears the readout; `fail` sets an
 * error directly (for flows, like delete, that don't need the de-race guard).
 */
export function useAsyncFeedback(materialGeneration?: unknown) {
  const generation = useRef(materialGeneration);
  const seq = useRef(0);
  const [material, setMaterial] = useState<{ generation: unknown; feedback: AsyncFeedback }>(
    () => ({
      generation: materialGeneration,
      feedback: { state: "idle" },
    }),
  );
  useLayoutEffect(() => {
    if (Object.is(generation.current, materialGeneration)) return;
    generation.current = materialGeneration;
    seq.current++;
  }, [materialGeneration]);
  const feedback = Object.is(material.generation, materialGeneration)
    ? material.feedback
    : ({ state: "idle" } satisfies AsyncFeedback);

  const publish = (next: AsyncFeedback) => {
    setMaterial({ generation: generation.current, feedback: next });
  };

  const reset = () => {
    seq.current++;
    publish({ state: "idle" });
  };

  const fail = (reason: string) => publish({ state: "error", reason });

  const run = async (
    op: () => Promise<{ ok: boolean; error?: string }>,
    fallback: string,
    ignoreError?: (error: unknown) => boolean,
  ) => {
    const token = ++seq.current;
    const admittedGeneration = generation.current;
    publish({ state: "busy" });
    try {
      const r = await op();
      if (seq.current !== token || !Object.is(generation.current, admittedGeneration)) return;
      publish(r.ok ? { state: "ok" } : { state: "error", reason: r.error ?? fallback });
    } catch (err) {
      if (seq.current !== token || !Object.is(generation.current, admittedGeneration)) return;
      if (ignoreError?.(err)) {
        publish({ state: "idle" });
        return;
      }
      fail(err instanceof Error ? err.message : fallback);
    }
  };

  return { feedback, reset, fail, run };
}
