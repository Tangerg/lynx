// useAgentSession owns the agent driver lifecycle for one session. The
// regression locked here: send() is re-entrancy-safe in the window before
// segment.started arrives. The steady-state guard reads the current root status
// only flips true a round-trip later, so without the synchronous `starting`
// latch a second Enter fires a second runs.start — two backend runs + an
// orphaned optimistic bubble whose localId is never relabeled.

import { act, renderHook, waitFor } from "@testing-library/react";
import { navigator } from "@/lib/navigation";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentDriver } from "@/plugins/sdk/types";
import {
  RpcError,
  type CancelRunResponse,
  type LyraClient,
  type RunEvent,
  type RunRef,
} from "@/rpc";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { agentTextInput } from "@/plugins/builtin/agent/domain/input";
import { resetContainer, setContainer } from "@/main/container";
import { useAgentStore } from "./agentStore";
import { useAgentSessionStore } from "./agentSessionStore";
import { useAgentSession } from "./useAgentSession";
import { selectCurrentRootRun } from "../application/view/runTree";

const SID = "ses_dbl";
const SID_B = "ses_next";

function autoPage<T>(data: T[]) {
  return { autoPagingToArray: vi.fn().mockResolvedValue(data) };
}

// A driver whose start() never resolves — keeps begin() parked in the
// pre-segment.started window where the latch is the only guard.
function parkedDriver(): { driver: AgentDriver; start: ReturnType<typeof vi.fn> } {
  const start = vi.fn(() => new Promise<never>(() => {}));
  const resume = vi.fn(() => new Promise<never>(() => {}));
  return { driver: { start, resume } as unknown as AgentDriver, start };
}

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(spec);
  // Mark draft so the effect skips history hydration (items.list → container).
  navigator().go({ session: SID });
  useAgentSessionStore.setState({
    draftSessionIds: new Set([SID]),
    freshDraftSessionIds: new Set([SID]),
  });
});
afterEach(() => {
  useAgentStore.getState().dropSession(SID);
  useAgentStore.getState().dropSession(SID_B);
  useAgentSessionStore.setState({
    draftSessionIds: new Set(),
    freshDraftSessionIds: new Set(),
  });
  resetContainer();
  vi.restoreAllMocks();
});

describe("useAgentSession driver lifecycle", () => {
  it("holds a directly mounted session open before lifecycle pruning can drop its view", () => {
    const { driver } = parkedDriver();
    useAgentSessionStore.setState({ openSessionIds: [], lastSessionId: "" });
    setContainer({
      client: () =>
        ({
          items: { list: vi.fn(() => autoPage([])) },
          interrupts: { list: vi.fn(() => autoPage([])) },
          plan: {
            get: vi.fn().mockResolvedValue({
              type: "plan",
              sessionId: SID,
              revision: 0,
              plan: [],
            }),
          },
          runs: { list: vi.fn(() => autoPage([])) },
        }) as unknown as LyraClient,
    });

    renderHook(() => useAgentSession(() => driver, SID));

    expect(useAgentSessionStore.getState().openSessionIds).toContain(SID);
    expect(useAgentSessionStore.getState().lastSessionId).toBe(SID);
    expect(useAgentStore.getState().sessions[SID]!.send).not.toBeNull();
  });

  it("uses session identity as the lifecycle key and the latest factory at that boundary", () => {
    const first = parkedDriver().driver;
    const second = parkedDriver().driver;
    const firstFactory = vi.fn(() => first);
    const secondFactory = vi.fn(() => second);
    useAgentSessionStore.setState({
      draftSessionIds: new Set([SID, SID_B]),
      freshDraftSessionIds: new Set([SID, SID_B]),
    });

    type HookProps = {
      makeDriver: () => AgentDriver;
      sessionId: string;
    };
    const { rerender } = renderHook(
      ({ makeDriver, sessionId }: HookProps) => useAgentSession(makeDriver, sessionId),
      { initialProps: { makeDriver: firstFactory, sessionId: SID } },
    );

    expect(firstFactory).toHaveBeenCalledTimes(1);

    rerender({ makeDriver: secondFactory, sessionId: SID });
    expect(secondFactory).not.toHaveBeenCalled();

    rerender({ makeDriver: secondFactory, sessionId: SID_B });
    expect(secondFactory).toHaveBeenCalledTimes(1);
    expect(useAgentStore.getState().sessions[SID]!.send).toBeNull();
    expect(useAgentStore.getState().sessions[SID_B]!.send).not.toBeNull();
  });
});

describe("useAgentSession send re-entrancy", () => {
  it("ignores a second send before the first run starts (no duplicate run/bubble)", () => {
    const { driver, start } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, SID));

    act(() => {
      const send = useAgentStore.getState().sessions[SID]!.send!;
      send(agentTextInput("first"));
      send(agentTextInput("second")); // blocked by the starting latch — still pre-segment.started
    });

    expect(start).toHaveBeenCalledTimes(1);
    const msgs = useAgentStore.getState().sessions[SID]!.view.messages;
    expect(msgs).toHaveLength(1);
    expect(msgs[0]!.blocks.some((b) => "text" in b && b.text === "first")).toBe(true);
  });
});

describe("useAgentSession run timing guards", () => {
  it("surfaces synchronous driver failures and releases the start latch", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const start = vi
      .fn()
      .mockImplementationOnce(() => {
        throw new RpcError({
          code: -32002,
          message: "session missing",
          data: { type: "session_not_found", detail: "gone" },
        });
      })
      .mockImplementationOnce(() => new Promise<never>(() => {}));
    const driver = {
      start,
      resume: vi.fn(() => new Promise<never>(() => {})),
    } as unknown as AgentDriver;
    renderHook(() => useAgentSession(() => driver, SID));

    act(() => {
      useAgentStore.getState().sessions[SID]!.send!(agentTextInput("first"));
    });

    await waitFor(() => {
      expect(useAgentStore.getState().sessions[SID]!.view.commandError).toMatchObject({
        message: "gone",
        code: "session_not_found",
      });
    });

    act(() => {
      useAgentStore.getState().sessions[SID]!.send!(agentTextInput("second"));
    });

    expect(start).toHaveBeenCalledTimes(2);
  });

  it("ignores a second resume while the first continuation is still starting", () => {
    const resume = vi.fn(() => new Promise<never>(() => {}));
    const driver = {
      start: vi.fn(() => new Promise<never>(() => {})),
      resume,
    } as unknown as AgentDriver;
    renderHook(() => useAgentSession(() => driver, SID));

    let firstAccepted = false;
    let secondAccepted = true;
    act(() => {
      const resumeAction = useAgentStore.getState().sessions[SID]!.resume!;
      firstAccepted = resumeAction("run_parent" as never, []);
      secondAccepted = resumeAction("run_parent" as never, []);
    });

    expect(resume).toHaveBeenCalledTimes(1);
    expect(firstAccepted).toBe(true);
    expect(secondAccepted).toBe(false);
  });

  it("keeps a run live until cancellation is committed by the runtime", async () => {
    const cancellation = deferred<CancelRunResponse>();
    const canceledRun = runRef({
      id: "run_resume",
      status: "finished",
      activeSegmentId: undefined,
      outcome: { type: "canceled" },
      finishedAt: "2026-07-30T02:00:01.000Z",
    });
    const cancel = vi.fn(() => cancellation.promise);
    setContainer({
      client: () =>
        ({
          items: { list: vi.fn(() => autoPage([])) },
          interrupts: { list: vi.fn(() => autoPage([])) },
          plan: {
            get: vi.fn().mockResolvedValue({
              type: "plan",
              sessionId: SID,
              revision: 0,
              plan: [],
            }),
          },
          runs: {
            cancel,
            list: vi.fn(() => autoPage([canceledRun])),
          },
        }) as unknown as LyraClient,
    });
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const onSettled = vi.fn();
    const onStartError = vi.fn();
    const resume = vi.fn((_: unknown, __: unknown, signal: AbortSignal) =>
      Promise.resolve({
        result: { runId: "run_resume", segmentId: "seg_resume" },
        events: abortRejectingEvents(signal),
      }),
    );
    const driver = {
      start: vi.fn(() => new Promise<never>(() => {})),
      resume,
    } as unknown as AgentDriver;
    renderHook(() => useAgentSession(() => driver, SID));

    act(() => {
      useAgentStore.getState().sessions[SID]!.resume!(
        "run_parent" as never,
        [],
        onSettled,
        onStartError,
      );
    });

    await waitFor(() => expect(onSettled).toHaveBeenCalledTimes(1));
    act(() => {
      useAgentStore.getState().applyRunEvents(SID, [startedRunEvent("run_resume", "seg_resume")]);
    });
    errorSpy.mockClear();

    act(() => {
      useAgentStore.getState().sessions[SID]!.stop?.();
    });

    await waitFor(() => expect(cancel).toHaveBeenCalledWith("run_resume"));
    expect(selectCurrentRootRun(useAgentStore.getState().sessions[SID]!.view)?.status).toBe(
      "running",
    );

    cancellation.resolve({ type: "root", run: canceledRun });
    await waitFor(() => {
      expect(selectCurrentRootRun(useAgentStore.getState().sessions[SID]!.view)?.status).toBe(
        "finished",
      );
    });

    expect(onStartError).not.toHaveBeenCalled();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it("preserves the run and surfaces a structured cancellation failure", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const cancel = vi.fn().mockRejectedValue(
      new RpcError({
        code: -32002,
        message: "stale",
        data: { type: "stale_segment", detail: "run already moved" },
      }),
    );
    setContainer({
      client: () => ({ runs: { cancel } }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, SID));
    act(() => {
      useAgentStore
        .getState()
        .applyRunEvents(SID, [startedRunEvent("run_failed_cancel", "seg_failed_cancel")]);
      useAgentStore.getState().sessions[SID]!.stop?.();
    });

    await waitFor(() => {
      expect(useAgentStore.getState().sessions[SID]!.view.commandError).toEqual({
        code: "stale_segment",
        message: "run already moved",
      });
    });
    expect(useAgentStore.getState().sessions[SID]!.view.runsById.run_failed_cancel).toMatchObject({
      status: "running",
      activeSegmentId: "seg_failed_cancel",
    });
  });
});

// Durable recovery (§10.2): opening a NON-draft session must rebuild unresolved
// HITL cards from interrupts.list and reattach to a still-running run via
// runs.subscribe — the two paths that make a reload survivable.
describe("useAgentSession durable recovery", () => {
  const RID = "ses_recover";

  const approvalInterrupt = {
    type: "approval" as const,
    itemId: "item_appr",
    runId: "run_int",
    payload: { tool: { name: "shell", arguments: { command: "rm -rf build" } } },
  };

  function stubClient(
    overrides: Record<string, unknown> = {},
    interruptOverrides: Record<string, unknown> = {},
    items: unknown[] = [],
  ) {
    const listItems = vi.fn(() => autoPage(items));
    const subscribe = vi.fn(() =>
      Promise.resolve({
        result: { runId: "run_live", segmentId: "seg_live" },
        // Parked stream — yields nothing, never ends (the run is "still going").
        events: (async function* () {
          yield* [];
          await new Promise<never>(() => {});
        })(),
      }),
    );
    setContainer({
      client: () =>
        ({
          items: { list: listItems },
          plan: {
            get: vi.fn().mockResolvedValue({
              type: "plan",
              sessionId: RID,
              revision: 0,
              plan: [],
              updatedAt: "2026-07-29T00:00:00Z",
            }),
          },
          interrupts: {
            list: vi.fn(() => autoPage([])),
            ...(interruptOverrides as object),
          },
          runs: {
            list: vi.fn(() => autoPage([])),
            subscribe,
            ...(overrides as object),
          },
        }) as unknown as LyraClient,
    });
    return { listItems, subscribe };
  }

  beforeEach(() => {
    // NOT a draft — recovery only runs for existing sessions.
    navigator().go({ session: RID });
    useAgentSessionStore.setState({
      draftSessionIds: new Set(),
      freshDraftSessionIds: new Set(),
    });
  });
  afterEach(() => {
    useAgentStore.getState().dropSession(RID);
    resetContainer();
  });

  it("rebuilds pending approval cards from interrupts.list", async () => {
    stubClient(
      {},
      {
        list: vi.fn(() =>
          autoPage([
            {
              rootRunId: "run_int",
              sessionId: RID,
              interrupts: [approvalInterrupt],
              createdAt: "2026-06-11T00:00:00Z",
            },
          ]),
        ),
      },
    );
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => {
      expect(useAgentStore.getState().sessions[RID]!.view.pendingInterrupts).toHaveLength(1);
    });
    const view = useAgentStore.getState().sessions[RID]!.view;
    expect(view.pendingInterrupts[0]!.runId).toBe("run_int");
    const approval = view.messages
      .flatMap((m) => m.blocks)
      .find((b) => b.kind === "approval" && b.itemId === "item_appr");
    expect(approval).toMatchObject({ status: "requires-action", runId: "run_int" });
    expect(selectCurrentRootRun(view)).toBeNull();
  });

  it("rehydrates a persisted empty draft without publishing it as an ordinary Session", async () => {
    useAgentSessionStore.setState({
      draftSessionIds: new Set([RID]),
      freshDraftSessionIds: new Set(),
    });
    const { listItems } = stubClient();
    const { driver } = parkedDriver();

    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => expect(listItems).toHaveBeenCalledOnce());
    expect(useAgentSessionStore.getState().draftSessionIds.has(RID)).toBe(true);
  });

  it("graduates a persisted draft when durable recovery finds conversation history", async () => {
    useAgentSessionStore.setState({
      draftSessionIds: new Set([RID]),
      freshDraftSessionIds: new Set(),
    });
    stubClient(
      {
        list: vi.fn(() =>
          autoPage([
            {
              id: "run_used_draft",
              sessionId: RID,
              status: "finished",
              outcome: { type: "completed" },
              createdAt: "2026-08-12T00:00:00Z",
              finishedAt: "2026-08-12T00:00:01Z",
              metrics: { steps: 1, activeDurationMillis: 1 },
              protocolProfile: { interruptTypes: [], requiredFeatures: [] },
            },
          ]),
        ),
      },
      {},
      [
        {
          id: "item_used_draft",
          runId: "run_used_draft",
          status: "completed",
          createdAt: "2026-08-12T00:00:00Z",
          type: "userMessage",
          content: [{ type: "text", text: "used elsewhere" }],
        },
      ],
    );
    const { driver } = parkedDriver();

    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() =>
      expect(useAgentStore.getState().sessions[RID]?.view.messages).toHaveLength(1),
    );
    expect(useAgentSessionStore.getState().draftSessionIds.has(RID)).toBe(false);
  });

  it("reattaches to a still-running root run via runs.subscribe", async () => {
    const { subscribe } = stubClient({
      list: vi.fn(() =>
        autoPage([
          {
            id: "run_sub",
            sessionId: RID,
            status: "waiting",
            createdAt: "2026-07-29T00:00:00Z",
            metrics: { steps: 1, activeDurationMillis: 5 },
            protocolProfile: { interruptTypes: [], requiredFeatures: [] },
            spawnedByItemId: "item_x",
            parentRunId: "run_live",
            rootRunId: "run_live",
          },
          // A running run always names the segment executing it, and recovery
          // subscribes to THAT segment rather than to whatever is live by then.
          {
            id: "run_live",
            sessionId: RID,
            status: "running",
            activeSegmentId: "seg_live",
            createdAt: "2026-07-29T00:00:00Z",
            metrics: { steps: 0, activeDurationMillis: 0 },
            protocolProfile: { interruptTypes: [], requiredFeatures: [] },
          },
        ]),
      ),
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => {
      expect(selectCurrentRootRun(useAgentStore.getState().sessions[RID]!.view)?.status).toBe(
        "running",
      );
    });
    expect(subscribe).toHaveBeenCalledTimes(1);
    expect(subscribe).toHaveBeenCalledWith(
      { runId: "run_live", segmentId: "seg_live" },
      expect.any(AbortSignal),
    );
    expect(selectCurrentRootRun(useAgentStore.getState().sessions[RID]!.view)?.id).toBe("run_live");
  });

  it("treats a run that finishes between snapshot and subscribe as synchronized", async () => {
    const warning = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const subscribe = vi.fn().mockRejectedValue(
      new RpcError({
        code: -32002,
        message: "run finished",
        data: { type: "run_finished" },
      }),
    );
    const running = runRef({
      id: "run_raced_terminal",
      sessionId: RID,
      activeSegmentId: "seg_raced_terminal",
    });
    const finished = runRef({
      ...running,
      status: "finished",
      activeSegmentId: undefined,
      outcome: { type: "completed" },
      finishedAt: "2026-07-29T00:00:01Z",
    });
    const list = vi
      .fn()
      .mockImplementationOnce(() => autoPage([running]))
      .mockImplementation(() => autoPage([finished]));
    stubClient({
      list,
      subscribe,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => {
      expect(
        useAgentStore.getState().sessions[RID]!.view.runsById.run_raced_terminal,
      ).toMatchObject({ status: "finished", outcome: { type: "completed" } });
    });

    expect(list).toHaveBeenCalledTimes(2);
    expect(subscribe).toHaveBeenCalledTimes(1);
    expect(warning).not.toHaveBeenCalled();
  });

  it("reattaches the root's new segment after a waiting child is canceled", async () => {
    let canceled = false;
    const rootBefore = runRef({
      id: "run_root",
      sessionId: RID,
      activeSegmentId: "seg_before",
    });
    const childBefore = runRef({
      id: "run_child",
      sessionId: RID,
      status: "waiting",
      activeSegmentId: undefined,
      parentRunId: "run_root",
      rootRunId: "run_root",
      spawnedByItemId: "item_spawn",
    });
    const rootAfter = runRef({
      id: "run_root",
      sessionId: RID,
      activeSegmentId: "seg_after",
    });
    const childAfter = runRef({
      ...childBefore,
      status: "finished",
      outcome: { type: "canceled" },
      finishedAt: "2026-07-30T02:00:02.000Z",
    });
    const subscribe = vi.fn((request: { runId: string; segmentId: string }, signal: AbortSignal) =>
      Promise.resolve({
        result: request,
        events: abortRejectingEvents(signal),
      }),
    );
    const cancel = vi.fn().mockImplementation(async () => {
      canceled = true;
      return { type: "child", run: childAfter, rootRun: rootAfter };
    });
    setContainer({
      client: () =>
        ({
          items: { list: vi.fn(() => autoPage([])) },
          interrupts: { list: vi.fn(() => autoPage([])) },
          plan: {
            get: vi.fn().mockResolvedValue({
              type: "plan",
              sessionId: RID,
              revision: 0,
              plan: [],
            }),
          },
          runs: {
            list: vi.fn(() =>
              autoPage(canceled ? [rootAfter, childAfter] : [rootBefore, childBefore]),
            ),
            subscribe,
            cancel,
          },
        }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => {
      expect(subscribe).toHaveBeenCalledWith(
        { runId: "run_root", segmentId: "seg_before" },
        expect.any(AbortSignal),
      );
    });
    act(() => {
      useAgentStore.getState().sessions[RID]!.cancelRun?.("run_child");
    });

    await waitFor(() => {
      expect(subscribe).toHaveBeenCalledWith(
        { runId: "run_root", segmentId: "seg_after" },
        expect.any(AbortSignal),
      );
    });
    expect(useAgentStore.getState().sessions[RID]!.view.runsById.run_child).toMatchObject({
      status: "finished",
      outcome: { type: "canceled" },
    });
  });
});

function runRef(partial: Partial<RunRef> = {}): RunRef {
  return {
    id: "run_default",
    sessionId: SID,
    status: "running",
    activeSegmentId: "seg_default",
    createdAt: "2026-07-30T02:00:00.000Z",
    metrics: { steps: 0, activeDurationMillis: 0 },
    protocolProfile: { interruptTypes: [], requiredFeatures: [] },
    ...partial,
  };
}

function startedRunEvent(runId: string, segmentId: string): RunEvent {
  return {
    eventId: `evt:${runId}:started`,
    runId,
    segmentId,
    timestamp: "2026-07-30T02:00:00.000Z",
    event: {
      type: "segment.started",
      run: runRef({ id: runId, activeSegmentId: segmentId }),
    },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((onResolve) => {
    resolve = onResolve;
  });
  return { promise, resolve };
}

function abortRejectingEvents(signal: AbortSignal): AsyncIterable<RunEvent> {
  return {
    [Symbol.asyncIterator]() {
      return {
        async next(): Promise<IteratorResult<RunEvent>> {
          await new Promise<never>((_, reject) => {
            if (signal.aborted) {
              reject(new Error("aborted"));
              return;
            }
            signal.addEventListener("abort", () => reject(new Error("aborted")), { once: true });
          });
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}
