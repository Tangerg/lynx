import type { Item, LyraClient, PendingInterruptSet, RunRef } from "@/rpc";
import { asRunId, asSegmentId, asSessionId, collectPages } from "@/rpc";
import type { RunStream } from "./agentRunPump";

interface AgentSessionRecoveryOptions {
  client: Pick<LyraClient, "items" | "runs" | "interrupts">;
  sessionId: string;
  isCancelled: () => boolean;
  hasInteracted: () => boolean;
  includeDescendants: boolean;
  applyCompletedItems: (items: Item[]) => void;
  applyRunSnapshots: (runs: RunRef[]) => void;
  applyPendingInterruptSets: (sets: PendingInterruptSet[]) => void;
  setAbortController: (controller: AbortController) => void;
  pump: (stream: RunStream, signal: AbortSignal) => Promise<void>;
  /** Re-read the session-scoped state through its recovery method. The run stream
   *  carries snapshots only to a follower, so a window that just opened holds none. */
  recoverState: () => Promise<void>;
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

  // Independent of everything else this reconstruction does: a state key this
  // runtime cannot serve, or a read that fails, must not cost the session its
  // transcript, its waiting sets, or its reattach.
  await options.recoverState().catch((err: unknown) => {
    if (!options.isCancelled())
      console.warn("[agent] session state recovery failed:", options.sessionId, err);
  });
  if (stale(options)) return;

  const open = await collectPages((cursor) =>
    options.client.interrupts.list({ sessionId: sid, cursor }),
  );
  if (stale(options)) return;
  options.applyPendingInterruptSets(open);

  const runs = await collectPages((cursor) =>
    options.client.runs.list({
      sessionId: sid,
      cursor,
      ...(options.includeDescendants ? { includeDescendants: true } : {}),
    }),
  );
  if (stale(options)) return;
  options.applyRunSnapshots(runs);
  const runningRoots = runs.filter((run) => run.status === "running" && !run.spawnedByItemId);
  if (runningRoots.length > 1) {
    throw new Error(
      `cannot recover session ${options.sessionId}: ${runningRoots.length} root runs are running`,
    );
  }
  const root = runningRoots[0];
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
  options.applyCompletedItems(items);
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
  // The subscribe response is the wire's own shape now, so its ids are plain
  // strings and this is their parse site — the same rule runtimeRunsGateway
  // follows for a run it opens. The head it captured travels with them: it is the
  // position this attach was taken at, and the cursor a reattach hands back if the
  // stream drops before a single event is folded.
  await options.pump(
    {
      result: {
        runId: asRunId(stream.result.runId),
        segmentId: asSegmentId(stream.result.segmentId),
        ...(stream.result.headEventId ? { headEventId: stream.result.headEventId } : {}),
      },
      events: stream.events,
    },
    ctrl.signal,
  );
}
