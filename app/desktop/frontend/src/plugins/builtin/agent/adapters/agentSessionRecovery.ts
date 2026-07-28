import type { LyraClient, RunEvent, RunId, RunRef, StreamingResult } from "@/rpc";
import { asRunId, asSessionId, collectPages } from "@/rpc";
import type { FoldEvent } from "./agentStore";

interface AgentSessionRecoveryOptions {
  client: Pick<LyraClient, "items" | "runs">;
  sessionId: string;
  isCancelled: () => boolean;
  hasInteracted: () => boolean;
  applyEvents: (events: FoldEvent[]) => void;
  setAbortController: (controller: AbortController) => void;
  pump: (stream: StreamingResult<{ runId: RunId }, RunEvent>, signal: AbortSignal) => Promise<void>;
}

export function startAgentSessionRecovery(options: AgentSessionRecoveryOptions): void {
  void recover(options).catch((err: unknown) => {
    if (!options.isCancelled())
      console.error("[agent] session recovery failed:", options.sessionId, err);
  });
}

function stale(options: AgentSessionRecoveryOptions): boolean {
  return options.isCancelled() || options.hasInteracted();
}

async function recover(options: AgentSessionRecoveryOptions): Promise<void> {
  const sid = asSessionId(options.sessionId);
  await replayHistory(options);
  if (stale(options)) return;

  const open = await collectPages((cursor) =>
    options.client.runs.listOpenInterrupts({ sessionId: sid, cursor }),
  );
  if (stale(options)) return;
  for (const oi of open) {
    options.applyEvents([
      {
        event: {
          type: "segment.started",
          run: { id: oi.runId, sessionId: oi.sessionId, createdAt: oi.createdAt },
        },
      },
      {
        event: {
          type: "segment.finished",
          outcome: { type: "interrupt", interrupts: oi.interrupts },
        },
      },
    ]);
  }

  const running = await collectPages((cursor) =>
    options.client.runs.list({ sessionId: sid, cursor }),
  );
  if (stale(options)) return;
  const root = running.find((run) => !run.spawnedByItemId);
  if (root) await attachRootRun(options, root);
}

async function replayHistory(options: AgentSessionRecoveryOptions): Promise<void> {
  const items = await collectPages((cursor) =>
    options.client.items.list({ sessionId: asSessionId(options.sessionId), cursor }),
  );
  if (stale(options) || items.length === 0) return;
  options.applyEvents(
    items.map((item): FoldEvent => ({ event: { type: "item.completed", item } })),
  );
}

async function attachRootRun(options: AgentSessionRecoveryOptions, run: RunRef): Promise<void> {
  const ctrl = new AbortController();
  options.setAbortController(ctrl);
  let stream: Awaited<ReturnType<typeof options.client.runs.subscribe>>;
  try {
    stream = await options.client.runs.subscribe(asRunId(run.id), ctrl.signal);
  } catch (err) {
    if (options.isCancelled() || ctrl.signal.aborted) return;
    console.warn("[agent] run reattach failed:", options.sessionId, err);
    void replayHistory(options).catch(() => undefined);
    return;
  }
  if (options.isCancelled() || ctrl.signal.aborted) return;
  // Stamp the CURRENT segment id (from the subscribe response) so the synthetic
  // segment.started keys the segment correctly — the replayed real segment.started then
  // carries the same segmentId and won't re-reset the streaming readout.
  options.applyEvents([
    { event: { type: "segment.started", run }, segmentId: stream.result.segmentId },
  ]);
  await options.pump(stream, ctrl.signal);
}
