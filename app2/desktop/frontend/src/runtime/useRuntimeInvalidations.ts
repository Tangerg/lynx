import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import type { RuntimeConnection, RuntimeEvent } from "@lyra/runtime-contract";

import {
  consumeRuntimeInvalidations,
  runtimeQueryKeys,
} from "./runtimeQueries";

export type RuntimeSyncState = "connecting" | "live" | "retrying" | "idle";

export function useRuntimeInvalidations(
  connection: RuntimeConnection,
  enabled: boolean,
): RuntimeSyncState {
  const queryClient = useQueryClient();
  const [state, setState] = useState<RuntimeSyncState>(
    enabled ? "connecting" : "idle",
  );

  useEffect(() => {
    if (!enabled) {
      setState("idle");
      return;
    }

    const controller = new AbortController();
    let retry = 0;
    const invalidate = (event: RuntimeEvent) => {
      const topics =
        event.type === "resync" ? (event.topics ?? []) : [event.type];
      if (
        topics.includes("sessions.changed") ||
        topics.includes("runs.changed")
      ) {
        void queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.sessions(connection),
        });
      }
      if (
        topics.includes("plan.changed") ||
        topics.includes("goals.changed") ||
        topics.includes("runs.changed") ||
        topics.includes("sessions.changed")
      ) {
        const sessionIds = event.sessionIds ?? [];
        if (sessionIds.length === 0) {
          void queryClient.invalidateQueries({
            queryKey: [...runtimeQueryKeys.scope(connection), "session"],
          });
        } else {
          for (const sessionId of sessionIds) {
            void queryClient.invalidateQueries({
              queryKey: runtimeQueryKeys.snapshot(connection, sessionId),
            });
          }
        }
      }
    };

    const run = async () => {
      while (!controller.signal.aborted) {
        setState(retry === 0 ? "connecting" : "retrying");
        try {
          await consumeRuntimeInvalidations(
            connection,
            controller.signal,
            () => {
              if (controller.signal.aborted) return;
              retry = 0;
              setState("live");
            },
            invalidate,
          );
        } catch (error) {
          if (controller.signal.aborted || isAbort(error)) return;
        }
        if (controller.signal.aborted) return;
        retry += 1;
        setState("retrying");
        await abortableDelay(
          Math.min(500 * 2 ** Math.min(retry - 1, 4), 8_000),
          controller.signal,
        );
      }
    };

    void run();
    return () => controller.abort();
  }, [connection, enabled, queryClient]);

  return state;
}

function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

function abortableDelay(milliseconds: number, signal: AbortSignal) {
  return new Promise<void>((resolve) => {
    const timer = window.setTimeout(finish, milliseconds);
    function finish() {
      window.clearTimeout(timer);
      signal.removeEventListener("abort", finish);
      resolve();
    }
    signal.addEventListener("abort", finish, { once: true });
  });
}
