import type {
  LyraClient,
  RunEvent,
  RunId,
  RunMetrics,
  RunProtocolProfile,
  RunRef,
  StreamingResult,
} from "@/rpc";
import { asRunId, asSegmentId, asSessionId, collectPages } from "@/rpc";
import type { FoldEvent } from "./agentStore";

interface AgentSessionRecoveryOptions {
  client: Pick<LyraClient, "items" | "runs" | "interrupts">;
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

// A pending interrupt set says what a run is waiting on, not what it spent, so
// these reconstructed frames report no accounting. Zero reads as "nothing
// reported" in the fold, which keeps whatever the readout already had instead of
// overwriting it with a number this path never learned.
const unreported: RunMetrics = { steps: 0, activeDurationMs: 0 };

// The same read says nothing about the contract the run was created under, and
// these frames are a reconstruction rather than something the runtime sent. Empty
// sets are what this path can honestly assert; a consumer that needs the real
// profile has to read the run itself instead of trusting a synthesized frame.
const unknownProfile: RunProtocolProfile = { requiredFeatures: [], interruptTypes: [] };

function stale(options: AgentSessionRecoveryOptions): boolean {
  return options.isCancelled() || options.hasInteracted();
}

async function recover(options: AgentSessionRecoveryOptions): Promise<void> {
  const sid = asSessionId(options.sessionId);
  await replayHistory(options);
  if (stale(options)) return;

  const open = await collectPages((cursor) =>
    options.client.interrupts.list({ sessionId: sid, cursor }),
  );
  if (stale(options)) return;
  for (const oi of open) {
    options.applyEvents([
      {
        event: {
          type: "segment.started",
          run: {
            id: oi.rootRunId,
            sessionId: oi.sessionId,
            createdAt: oi.createdAt,
            metrics: unreported,
            protocolProfile: unknownProfile,
          },
        },
      },
      {
        event: {
          type: "segment.finished",
          outcome: { type: "interrupt", interrupts: oi.interrupts },
          metrics: unreported,
        },
      },
    ]);
  }

  // runs.list is the whole history now, so recovery has to say which part it means:
  // a run it can still attach to. Only a RUNNING root has a stream — a waiting one
  // is already reconstructed from the interrupt sets above, and subscribing to it
  // would be refused as run_waiting.
  const active = await collectPages((cursor) =>
    options.client.runs.list({ sessionId: sid, cursor, statuses: ["running"] }),
  );
  if (stale(options)) return;
  const root = active.find((run) => !run.spawnedByItemId);
  if (root) await attachRootRun(options, root);
}

async function replayHistory(options: AgentSessionRecoveryOptions): Promise<void> {
  const items = await collectPages((cursor) =>
    options.client.items.list({
      scope: { type: "session", sessionId: asSessionId(options.sessionId) },
      cursor,
    }),
  );
  if (stale(options) || items.length === 0) return;
  options.applyEvents(
    items.map((item): FoldEvent => ({ event: { type: "item.completed", item } })),
  );
}

async function attachRootRun(options: AgentSessionRecoveryOptions, run: RunRef): Promise<void> {
  // A running root always names its segment; without one there is nothing to
  // attach to, and asking for "whatever is live" is exactly what the protocol
  // stopped allowing.
  if (!run.activeSegmentId) return;
  const ctrl = new AbortController();
  options.setAbortController(ctrl);
  let stream: Awaited<ReturnType<typeof options.client.runs.subscribe>>;
  try {
    stream = await options.client.runs.subscribe(
      { runId: asRunId(run.id), segmentId: asSegmentId(run.activeSegmentId) },
      ctrl.signal,
    );
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
    { event: { type: "segment.started", run }, segmentId: asSegmentId(stream.result.segmentId) },
  ]);
  // The subscribe response is the wire's own shape now, so its ids are plain
  // strings and this is their parse site — the same rule runtimeRunsGateway
  // follows for a run it opens.
  await options.pump(
    { result: { runId: asRunId(stream.result.runId) }, events: stream.events },
    ctrl.signal,
  );
}
