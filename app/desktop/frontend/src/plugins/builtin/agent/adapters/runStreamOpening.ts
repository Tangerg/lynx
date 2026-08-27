import type { ScopeAppClient } from "@/rpc";

type RuntimeRunStream = Awaited<ReturnType<ScopeAppClient["runs"]["subscribe"]>>;

/**
 * Give an aborting generation immediate ownership release even when the
 * transport opening ignores its signal. A stream which arrives after that
 * release is still an acquired foreign resource and must be retired.
 */
export function settleRunStreamOpening(
  opening: Promise<RuntimeRunStream>,
  signal: AbortSignal,
): Promise<RuntimeRunStream | null> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const onAbort = () => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", onAbort);
      resolve(null);
    };
    if (signal.aborted) onAbort();
    else signal.addEventListener("abort", onAbort, { once: true });

    void opening.then(
      (stream) => {
        if (settled) {
          retireRunStream(stream);
          return;
        }
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(stream);
      },
      (error: unknown) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

export function retireRunStream(stream: RuntimeRunStream): void {
  try {
    const closing = stream.events[Symbol.asyncIterator]().return?.();
    if (closing) void Promise.resolve(closing).catch(() => undefined);
  } catch {
    // The generation is already fenced. Abort remains the authoritative
    // teardown path when a foreign iterator cannot be constructed or closed.
  }
}
