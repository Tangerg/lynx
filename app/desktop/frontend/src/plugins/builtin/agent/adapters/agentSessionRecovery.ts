import type { LyraClient } from "@/rpc";
import { asRunId, asSegmentId, RpcConnectionError } from "@/rpc";
import type { AgentRunView, AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { refreshAgentSessionProjection } from "../application/session/refreshSessionProjection";
import { agentRuntime } from "../application/ports/runtimeGateway";
import type { RunStream } from "./agentRunPump";

interface AgentSessionRecoveryOptions {
  client: Pick<LyraClient, "runs">;
  sessionId: string;
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
    if (!options.isCancelled()) {
      console.error("[agent] session recovery failed:", options.sessionId, error);
    }
    return null;
  });
}

function stale(options: AgentSessionRecoveryOptions): boolean {
  return options.isCancelled() || options.hasInteracted();
}

async function recover(options: AgentSessionRecoveryOptions): Promise<AgentSessionView | null> {
  const view = await refreshAgentSessionProjection(options.sessionId, {
    canCommit: () => !stale(options),
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
  options.setAbortController(controller);
  let stream: Awaited<ReturnType<typeof options.client.runs.subscribe>>;
  try {
    stream = await options.client.runs.subscribe(
      { runId: asRunId(run.id), segmentId: asSegmentId(segmentId) },
      controller.signal,
    );
  } catch (error) {
    if (options.isCancelled() || controller.signal.aborted) return;
    if (agentRuntime().isRunGone(error)) {
      await refreshAgentSessionProjection(options.sessionId, {
        canCommit: () => !stale(options),
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
}
