import type { LyraClient, RunEvent, RunId, RunRef, StreamingResult } from "@/rpc";
import { asSessionId } from "@/rpc";
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

  const open = await options.client.runs.listOpenInterrupts(sid);
  if (stale(options)) return;
  for (const oi of open.data) {
    options.applyEvents([
      {
        event: {
          type: "run.started",
          run: { id: oi.parentRunId, sessionId: oi.sessionId, createdAt: oi.createdAt },
        },
      },
      {
        event: {
          type: "run.finished",
          outcome: { type: "interrupt", interrupts: oi.interrupts },
        },
      },
    ]);
  }

  const running = await options.client.runs.list(sid);
  if (stale(options)) return;
  const root = running.data.find((run) => !run.spawnedByItemId);
  if (root) await attachRootRun(options, root);
}

async function replayHistory(options: AgentSessionRecoveryOptions): Promise<void> {
  const resp = await options.client.items.list({ sessionId: asSessionId(options.sessionId) });
  if (stale(options) || resp.data.length === 0) return;
  options.applyEvents(
    resp.data.map((item): FoldEvent => ({ event: { type: "item.completed", item } })),
  );
}

async function attachRootRun(options: AgentSessionRecoveryOptions, run: RunRef): Promise<void> {
  const ctrl = new AbortController();
  options.setAbortController(ctrl);
  let stream: StreamingResult<{ runId: RunId }, RunEvent>;
  try {
    stream = await options.client.runs.subscribe(run.id, ctrl.signal);
  } catch (err) {
    if (options.isCancelled() || ctrl.signal.aborted) return;
    console.warn("[agent] run reattach failed:", options.sessionId, err);
    void replayHistory(options).catch(() => undefined);
    return;
  }
  if (options.isCancelled() || ctrl.signal.aborted) return;
  options.applyEvents([{ event: { type: "run.started", run } }]);
  await options.pump(stream, ctrl.signal);
}
