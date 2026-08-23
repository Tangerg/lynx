import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import type {
  RuntimeConnection,
  RuntimeEvent,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import {
  consumeRuntimeInvalidations,
  runtimeQueryKeys,
} from "./runtimeQueries";

export type RuntimeSyncState = "connecting" | "live" | "retrying" | "idle";

export function useRuntimeInvalidations(
  connection: RuntimeConnection,
  enabled: boolean,
  watch?: { id: string; workspace: WorkspaceRef },
): RuntimeSyncState {
  const queryClient = useQueryClient();
  const watchId = watch?.id;
  const watchWorkspacePath = watch?.workspace.path;
  const [state, setState] = useState<RuntimeSyncState>(
    enabled ? "connecting" : "idle",
  );

  useEffect(() => {
    if (!enabled) {
      setState("idle");
      return;
    }

    const controller = new AbortController();
    const activeWatch =
      watchId === undefined || watchWorkspacePath === undefined
        ? undefined
        : { id: watchId, workspace: { path: watchWorkspacePath } };
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
      if (topics.includes("models.changed")) {
        void queryClient.invalidateQueries({
          queryKey: [...runtimeQueryKeys.scope(connection), "models"],
        });
        void queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.providers(connection),
        });
        void queryClient.invalidateQueries({
          queryKey: [...runtimeQueryKeys.scope(connection), "model-role"],
        });
      }
      if (topics.includes("mcp.changed")) {
        void queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.mcp(connection),
        });
      }
      if (topics.includes("approvals.changed")) {
        void queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.approvals(connection),
        });
      }
      if (topics.includes("schedules.changed")) {
        void queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.schedules(connection),
        });
      }
      if (topics.includes("hooks.changed")) {
        void queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.hooks(connection),
        });
      }
      if (topics.includes("files.changed")) {
        const workspace = event.workspace ?? activeWatch?.workspace;
        if (workspace !== undefined) {
          void queryClient.invalidateQueries({
            queryKey: runtimeQueryKeys.workspace(connection, workspace.path),
          });
        }
      }
      if (topics.includes("skills.changed")) {
        void queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.skills(connection),
        });
      }
      if (topics.includes("knowledge.changed")) {
        void queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.knowledge(connection),
        });
      }
      if (topics.includes("agentMemory.changed")) {
        void queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.memory(connection),
        });
      }
      if (topics.includes("codebase.changed")) {
        void queryClient.invalidateQueries({
          queryKey: [...runtimeQueryKeys.scope(connection), "codebase"],
        });
      }
      if (
        topics.includes("plan.changed") ||
        topics.includes("goals.changed") ||
        topics.includes("interrupts.changed") ||
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
            void queryClient.invalidateQueries({
              queryKey: runtimeQueryKeys.sessionHistory(connection, sessionId),
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
            activeWatch,
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
  }, [connection, enabled, queryClient, watchId, watchWorkspacePath]);

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
