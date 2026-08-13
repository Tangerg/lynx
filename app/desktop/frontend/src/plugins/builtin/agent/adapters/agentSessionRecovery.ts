import type { LyraClient } from "@/rpc";
import { asRunId, asSegmentId, RpcConnectionError } from "@/rpc";
import type { AgentRunView, AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { refreshAgentSessionProjection } from "../application/session/refreshSessionProjection";
import { agentRuntime } from "../application/ports/runtimeGateway";
import type { RunStream } from "./agentRunPump";

const ABORTED = Symbol("agent-session-reattach.aborted");

interface AgentSessionRecoveryOptions {
  client: Pick<LyraClient, "runs">;
  sessionId: string;
  signal: AbortSignal;
  isCancelled: () => boolean;
  hasInteracted: () => boolean;
  isFollowing: (runId: string, segmentId: string) => boolean;
  setAbortController: (controller: AbortController) => void;
  pump: (stream: RunStream, signal: AbortSignal) => Promise<void>;
}

export function startAgentSessionRecovery(
  options: AgentSessionRecoveryOptions,
): Promise<AgentSessionView | null> {
  return recover(options).catch((error: unknown) => {
    if (!options.signal.aborted && !options.isCancelled()) {
      console.error("[agent] session recovery failed:", options.sessionId, error);
    }
    return null;
  });
}

function stale(options: AgentSessionRecoveryOptions): boolean {
  return options.signal.aborted || options.isCancelled() || options.hasInteracted();
}

async function recover(options: AgentSessionRecoveryOptions): Promise<AgentSessionView | null> {
  const view = await refreshAgentSessionProjection(options.sessionId, {
    canCommit: () => !stale(options),
    signal: options.signal,
  });
  if (!view || stale(options)) return null;

  const runningRoots = Object.values(view.runsById).filter(
    (run) => run.parentRunId === null && run.status === "running",
  );
  if (runningRoots.length > 1) {
    throw new Error(
      `cannot synchronize agent session ${options.sessionId}: durable snapshot contains ${runningRoots.length} running root runs (${runningRoots.map((run) => run.id).join(", ")}); expected at most one`,
    );
  }
  const root = runningRoots[0];
  if (root) await attachRootRun(options, root);
  return view;
}

async function attachRootRun(
  options: AgentSessionRecoveryOptions,
  run: AgentRunView,
): Promise<void> {
  const segmentId = run.activeSegmentId;
  if (!segmentId || options.isFollowing(run.id, segmentId)) return;

  const controller = new AbortController();
  const abort = () => controller.abort(options.signal.reason);
  if (options.signal.aborted) abort();
  else options.signal.addEventListener("abort", abort, { once: true });
  options.setAbortController(controller);
  try {
    let stream: Awaited<ReturnType<typeof options.client.runs.subscribe>>;
    try {
      const opening = options.client.runs.subscribe(
        { runId: asRunId(run.id), segmentId: asSegmentId(segmentId) },
        controller.signal,
      );
      const opened = await settleBeforeAbort(opening, controller.signal, disposeRunStream);
      if (opened === ABORTED) return;
      stream = opened;
    } catch (error) {
      if (options.isCancelled() || controller.signal.aborted) return;
      if (agentRuntime().isRunGone(error)) {
        await refreshAgentSessionProjection(options.sessionId, {
          canCommit: () => !stale(options),
          signal: options.signal,
        });
        return;
      }
      if (error instanceof RpcConnectionError) return;
      console.warn("[agent] run reattach failed:", options.sessionId, error);
      return;
    }
    if (options.isCancelled() || controller.signal.aborted) return;
    await options.pump(
      {
        result: {
          runId: asRunId(stream.result.runId),
          segmentId: asSegmentId(stream.result.segmentId),
          ...(stream.result.headEventId ? { headEventId: stream.result.headEventId } : {}),
        },
        events: stream.events,
      },
      controller.signal,
    );
  } finally {
    options.signal.removeEventListener("abort", abort);
  }
}

function settleBeforeAbort<T>(
  operation: Promise<T>,
  signal: AbortSignal,
  disposeLateValue: (value: T) => void,
): Promise<T | typeof ABORTED> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const onAbort = () => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", onAbort);
      resolve(ABORTED);
    };
    if (signal.aborted) onAbort();
    else signal.addEventListener("abort", onAbort, { once: true });
    operation.then(
      (value) => {
        if (settled) {
          disposeLateValue(value);
          return;
        }
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(value);
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

function disposeRunStream(stream: Awaited<ReturnType<LyraClient["runs"]["subscribe"]>>): void {
  try {
    const closing = stream.events[Symbol.asyncIterator]().return?.();
    if (closing) void Promise.resolve(closing).catch(() => undefined);
  } catch {
    // The reattach generation is already fenced. Abort remains authoritative
    // when a foreign iterable cannot be constructed or closed.
  }
}
