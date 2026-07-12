import { queryClient } from "@/lib/data/queryClient";
import type { RunEvent, RunId, StreamingResult } from "@/rpc";
import { AGENT_SESSION_USAGE_KEY } from "../application/session/sessionUsage";
import type { FoldEvent } from "./agentStore";
import { createRunEventBatcher } from "./runEventBatcher";

interface AgentRunPumpOptions {
  sessionId: string;
  isCancelled: () => boolean;
  readEpoch: () => number;
  applyEvents: (events: FoldEvent[]) => void;
}

interface AgentRunPump {
  pump: (stream: StreamingResult<{ runId: RunId }, RunEvent>, signal: AbortSignal) => Promise<void>;
  cancelCurrentRun: (cancel: (runId: RunId) => Promise<void>) => void;
  dispose: () => void;
}

export function createAgentRunPump({
  sessionId,
  isCancelled,
  readEpoch,
  applyEvents,
}: AgentRunPumpOptions): AgentRunPump {
  let currentRunId: RunId | null = null;

  const eventBatcher = createRunEventBatcher({
    readEpoch,
    apply: applyEvents,
    onRunFinished: () => {
      void queryClient.invalidateQueries({ queryKey: [AGENT_SESSION_USAGE_KEY, sessionId] });
    },
  });

  return {
    async pump(stream, signal) {
      const runId = stream.result.runId;
      currentRunId = runId;
      try {
        for await (const ev of stream.events) {
          if (isCancelled() || signal.aborted) break;
          eventBatcher.enqueue(ev.event, ev.runId, ev.segmentId);
        }
      } catch (err) {
        if (!isCancelled() && !signal.aborted)
          console.error("[agent] run stream failed:", sessionId, err);
      } finally {
        if (currentRunId === runId) currentRunId = null;
      }
    },
    cancelCurrentRun(cancel) {
      if (currentRunId) void cancel(currentRunId).catch(() => undefined);
    },
    dispose() {
      eventBatcher.dispose();
    },
  };
}
