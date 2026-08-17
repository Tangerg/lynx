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
import { agentTextInput } from "@/plugins/builtin/agent/domain/input";
import { resetContainer, setContainer } from "@/main/container";
import { useAgentStore } from "./agentStore";
import { useAgentSessionStore } from "./agentSessionStore";
import { useAgentSession } from "./useAgentSession";
import { selectCurrentRootRun } from "../application/view/runTree";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

const SID = "ses_dbl";
const SID_B = "ses_next";

function parkUntilAborted(signal: AbortSignal): Promise<never> {
  return new Promise<never>((_resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason);
      return;
    }
    signal.addEventListener("abort", () => reject(signal.reason), { once: true });
  });
}

// A driver whose start() never resolves — keeps begin() parked in the
// pre-segment.started window where the latch is the only guard.
function parkedDriver(): { driver: AgentDriver; start: ReturnType<typeof vi.fn> } {
  const start = vi.fn((_input: unknown, _options: unknown, signal: AbortSignal) =>
    parkUntilAborted(signal),
  );
  const resume = vi.fn((_runId: unknown, _responses: unknown, signal: AbortSignal) =>
    parkUntilAborted(signal),
  );
  return { driver: { start, resume } as unknown as AgentDriver, start };
}

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(spec);
  // Mark draft so the effect skips history hydration (items.list → container).
  navigator().go({ session: SID });
  useAgentSessionStore.setState({
    draftSessionIds: new Set([SID]),
    freshDraftSessionIds: new Set([SID]),
  });
});
afterEach(async () => {
  useAgentStore.getState().dropSession(SID);
  useAgentStore.getState().dropSession(SID_B);
  useAgentSessionStore.setState({
    draftSessionIds: new Set(),
    freshDraftSessionIds: new Set(),
  });
  await resetContainer();
  vi.restoreAllMocks();
});

describe("useAgentSession driver lifecycle", () => {
  it("holds a directly mounted session open before lifecycle pruning can drop its view", () => {
    const { driver } = parkedDriver();
    useAgentSessionStore.setState({ openSessionIds: [], lastSessionId: "" });
    setContainer({
      client: () =>
        ({
          sessions: {
            snapshot: vi.fn().mockResolvedValue({
              items: [],
              runs: [],
              interrupts: [],
              state: {
                type: "plan",
                sessionId: SID,
                revision: 0,
                plan: [],
              },
            }),
          },
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

  it("rejects a fresh send while the current root is parked for HITL", () => {
    const { driver, start } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, SID));
    act(() => {
      useAgentStore
        .getState()
        .applyRunSnapshot(SID, runRef({ status: "waiting", activeSegmentId: undefined }));
    });

    let accepted = true;
    act(() => {
      accepted = useAgentStore.getState().sessions[SID]!.send!(
        agentTextInput("must wait for the interrupt"),
      );
    });

    expect(accepted).toBe(false);
    expect(start).not.toHaveBeenCalled();
    expect(useAgentStore.getState().sessions[SID]!.view.messages).toHaveLength(0);
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
      .mockImplementationOnce((...args: unknown[]) => parkUntilAborted(args.at(-1) as AbortSignal));
    const driver = {
      start,
      resume: vi.fn((_runId: unknown, _responses: unknown, signal: AbortSignal) =>
        parkUntilAborted(signal),
      ),
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
    const resume = vi.fn((_runId: unknown, _responses: unknown, signal: AbortSignal) =>
      parkUntilAborted(signal),
    );
    const driver = {
      start: vi.fn((_input: unknown, _options: unknown, signal: AbortSignal) =>
        parkUntilAborted(signal),
      ),
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
          sessions: {
            snapshot: vi.fn().mockResolvedValue({
              items: [],
              runs: [canceledRun],
              interrupts: [],
              state: { type: "plan", sessionId: SID, revision: 0, plan: [] },
            }),
          },
          runs: {
            cancel,
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
      start: vi.fn((_input: unknown, _options: unknown, signal: AbortSignal) =>
        parkUntilAborted(signal),
      ),
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

  it("does not fold a cancellation response after its Runtime generation retires", async () => {
    const cancellation = deferred<CancelRunResponse>();
    const cancel = vi.fn(() => cancellation.promise);
    setContainer({
      client: () => ({ runs: { cancel } }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, SID));
    act(() => {
      useAgentStore
        .getState()
        .applyRunEvents(SID, [startedRunEvent("run-retired-cancel", "seg-retired-cancel")]);
      useAgentStore.getState().sessions[SID]!.cancelRun?.("run-retired-cancel");
    });
    await waitFor(() => expect(cancel).toHaveBeenCalledWith("run-retired-cancel"));

    act(() => {
      const store = useAgentStore.getState();
      store.retireProjectionGeneration([SID]);
      void store.sessions[SID]!.synchronize?.("retire-live");
    });

    await act(async () => {
      cancellation.resolve({
        type: "root",
        run: runRef({
          id: "run-retired-cancel",
          status: "finished",
          activeSegmentId: undefined,
          outcome: { type: "canceled" },
          finishedAt: "2026-07-30T02:00:02.000Z",
        }),
      });
      await cancellation.promise;
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(
      useAgentStore.getState().sessions[SID]!.view.runsById["run-retired-cancel"],
    ).toMatchObject({ status: "running", activeSegmentId: "seg-retired-cancel" });
  });

  it("admits successor cancellation only through the replacement Runtime client", async () => {
    const predecessorCancellation = deferred<CancelRunResponse>();
    const successorCancellation = deferred<CancelRunResponse>();
    const predecessorCancel = vi.fn(() => predecessorCancellation.promise);
    let successorAccepted = false;
    const successorCancel = vi.fn(async () => {
      const response = await successorCancellation.promise;
      successorAccepted = true;
      return response;
    });
    setContainer({
      client: () => ({ runs: { cancel: predecessorCancel } }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, SID));
    act(() => {
      useAgentStore
        .getState()
        .applyRunEvents(SID, [startedRunEvent("run-replaced-cancel", "seg-replaced-cancel")]);
      useAgentStore.getState().sessions[SID]!.cancelRun?.("run-replaced-cancel");
    });
    await waitFor(() => expect(predecessorCancel).toHaveBeenCalledOnce());

    act(() => {
      const store = useAgentStore.getState();
      store.retireProjectionGeneration([SID]);
      void store.sessions[SID]!.synchronize?.("retire-live");
    });
    const restoredRun = runRef({
      id: "run-replaced-cancel",
      status: "waiting",
      activeSegmentId: undefined,
    });
    setContainer({
      client: () =>
        ({
          sessions: {
            snapshot: vi.fn().mockImplementation(() =>
              Promise.resolve({
                items: [],
                runs: [
                  successorAccepted
                    ? runRef({
                        id: "run-replaced-cancel",
                        status: "finished",
                        activeSegmentId: undefined,
                        outcome: { type: "canceled" },
                        finishedAt: "2026-07-30T02:00:03.000Z",
                      })
                    : restoredRun,
                ],
                interrupts: [],
                state: { type: "plan", sessionId: SID, revision: 0, plan: [] },
              }),
            ),
          },
          runs: { cancel: successorCancel },
        }) as unknown as LyraClient,
    });

    let replacement!: Promise<boolean>;
    act(() => {
      const store = useAgentStore.getState();
      store.retireProjectionGeneration([SID]);
      replacement = store.sessions[SID]!.synchronize!("replace-live");
    });
    await expect(replacement).resolves.toBe(true);
    act(() => {
      useAgentStore.getState().sessions[SID]!.cancelRun?.("run-replaced-cancel");
    });
    await waitFor(() => expect(successorCancel).toHaveBeenCalledOnce());

    await act(async () => {
      predecessorCancellation.resolve({
        type: "root",
        run: runRef({
          id: "run-replaced-cancel",
          status: "finished",
          activeSegmentId: undefined,
          outcome: { type: "canceled" },
          finishedAt: "2026-07-30T02:00:02.000Z",
        }),
      });
      await predecessorCancellation.promise;
      await Promise.resolve();
    });
    expect(
      useAgentStore.getState().sessions[SID]!.view.runsById["run-replaced-cancel"],
    ).toMatchObject({ status: "waiting", activeSegmentId: null });

    successorCancellation.resolve({
      type: "root",
      run: runRef({
        id: "run-replaced-cancel",
        status: "finished",
        activeSegmentId: undefined,
        outcome: { type: "canceled" },
        finishedAt: "2026-07-30T02:00:03.000Z",
      }),
    });
    await waitFor(() =>
      expect(
        useAgentStore.getState().sessions[SID]!.view.runsById["run-replaced-cancel"],
      ).toMatchObject({ status: "finished", outcome: { type: "canceled" } }),
    );
    expect(predecessorCancel).toHaveBeenCalledOnce();
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

  it("converges without an error when another client terminates the Run first", async () => {
    const terminal = runRef({
      id: "run_remote_terminal",
      status: "finished",
      activeSegmentId: undefined,
      outcome: { type: "completed" },
      finishedAt: "2026-07-30T02:00:02.000Z",
    });
    const cancel = vi.fn().mockRejectedValue(
      new RpcError({
        code: -32002,
        message: "already finished",
        data: { type: "run_finished" },
      }),
    );
    setContainer({
      client: () =>
        ({
          sessions: {
            snapshot: vi.fn().mockResolvedValue({
              items: [],
              runs: [terminal],
              interrupts: [],
              state: { type: "plan", sessionId: SID, revision: 0, plan: [] },
            }),
          },
          runs: {
            cancel,
          },
        }) as unknown as LyraClient,
    });
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, SID));
    act(() => {
      useAgentStore
        .getState()
        .applyRunEvents(SID, [startedRunEvent("run_remote_terminal", "seg_remote")]);
      useAgentStore.getState().sessions[SID]!.stop?.();
    });

    await waitFor(() => {
      expect(
        useAgentStore.getState().sessions[SID]!.view.runsById.run_remote_terminal,
      ).toMatchObject({ status: "finished", outcome: { type: "completed" } });
    });
    expect(useAgentStore.getState().sessions[SID]!.view.commandError).toBeNull();
    expect(errorSpy).not.toHaveBeenCalled();
  });
});

// Durable recovery (§10.2): opening a NON-draft session must rebuild one coherent
// material snapshot and reattach to a still-running run via runs.subscribe.
describe("useAgentSession durable recovery", () => {
  const RID = "ses_recover";

  const approvalInterrupt = {
    type: "approval" as const,
    itemId: "item_appr",
    runId: "run_int",
    payload: { tool: { name: "shell", arguments: { command: "rm -rf build" } } },
  };

  function materialSnapshot(overrides: Record<string, unknown> = {}) {
    return {
      items: [],
      runs: [],
      interrupts: [],
      state: {
        type: "plan",
        sessionId: RID,
        revision: 0,
        plan: [],
        updatedAt: "2026-07-29T00:00:00Z",
      },
      ...overrides,
    };
  }

  function stubClient(
    runOverrides: Record<string, unknown> = {},
    snapshotOverrides: Record<string, unknown> = {},
  ) {
    const readSnapshot = vi.fn().mockResolvedValue(materialSnapshot(snapshotOverrides));
    const subscribe = vi.fn((_params: unknown, signal: AbortSignal) =>
      Promise.resolve({
        result: { runId: "run_live", segmentId: "seg_live" },
        // Parked while the run remains live, but owned by the subscribe signal
        // so hook teardown settles the iterator instead of leaking it.
        events: abortRejectingEvents(signal),
      }),
    );
    setContainer({
      client: () =>
        ({
          sessions: { snapshot: readSnapshot },
          runs: {
            subscribe,
            ...(runOverrides as object),
          },
        }) as unknown as LyraClient,
    });
    return { readSnapshot, subscribe };
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

  it("rebuilds pending approval cards from the material snapshot", async () => {
    stubClient(
      {},
      {
        interrupts: [
          {
            rootRunId: "run_int",
            sessionId: RID,
            interrupts: [approvalInterrupt],
            createdAt: "2026-06-11T00:00:00Z",
          },
        ],
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
    const { readSnapshot } = stubClient();
    const { driver } = parkedDriver();

    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => expect(readSnapshot).toHaveBeenCalledOnce());
    expect(useAgentSessionStore.getState().draftSessionIds.has(RID)).toBe(true);
  });

  it("graduates a persisted draft when durable recovery finds conversation history", async () => {
    useAgentSessionStore.setState({
      draftSessionIds: new Set([RID]),
      freshDraftSessionIds: new Set(),
    });
    stubClient(
      {},
      {
        runs: [
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
        ],
        items: [
          {
            id: "item_used_draft",
            runId: "run_used_draft",
            status: "completed",
            createdAt: "2026-08-12T00:00:00Z",
            type: "userMessage",
            content: [{ type: "text", text: "used elsewhere" }],
          },
        ],
      },
    );
    const { driver } = parkedDriver();

    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() =>
      expect(useAgentStore.getState().sessions[RID]?.view.messages).toHaveLength(1),
    );
    expect(useAgentSessionStore.getState().draftSessionIds.has(RID)).toBe(false);
  });

  it("reattaches to a still-running root run via runs.subscribe", async () => {
    const { subscribe } = stubClient(
      {},
      {
        runs: [
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
        ],
      },
    );
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

  it("replaces a non-cooperative live generation before Runtime restart reconciliation", async () => {
    let restarted = false;
    const closeOldStream = vi.fn(async () => ({ value: undefined, done: true }) as const);
    let releaseOldNext!: (result: IteratorResult<RunEvent>) => void;
    const readSnapshot = vi.fn().mockImplementation(() =>
      Promise.resolve(
        materialSnapshot({
          items: restarted
            ? [
                {
                  id: "item_after_restart",
                  runId: "run_after_restart",
                  type: "toolCall",
                  status: "running",
                  startedAt: "2026-08-13T00:00:01.000Z",
                  tool: { name: "shell", arguments: { command: "npm test" } },
                },
              ]
            : [],
          runs: [
            restarted
              ? runRef({
                  id: "run_after_restart",
                  sessionId: RID,
                  status: "waiting",
                  activeSegmentId: undefined,
                })
              : runRef({
                  id: "run_before_restart",
                  sessionId: RID,
                  activeSegmentId: "seg_before_restart",
                }),
          ],
          interrupts: restarted
            ? [
                {
                  rootRunId: "run_after_restart",
                  sessionId: RID,
                  createdAt: "2026-08-13T00:00:02.000Z",
                  interrupts: [
                    {
                      type: "approval",
                      itemId: "item_after_restart",
                      runId: "run_after_restart",
                      payload: {
                        tool: { name: "shell", arguments: { command: "npm test" } },
                      },
                    },
                  ],
                },
              ]
            : [],
          state: {
            type: "plan",
            sessionId: RID,
            revision: restarted ? 2 : 1,
            plan: [
              {
                id: restarted ? "step_after_restart" : "step_before_restart",
                description: restarted ? "Approve resumed tool" : "Run old generation",
                status: "in_progress",
              },
            ],
          },
        }),
      ),
    );
    const subscribe = vi.fn(() =>
      Promise.resolve({
        result: { runId: "run_before_restart", segmentId: "seg_before_restart" },
        events: {
          [Symbol.asyncIterator]: () => ({
            next: () =>
              new Promise<IteratorResult<RunEvent>>((resolve) => {
                releaseOldNext = resolve;
              }),
            return: closeOldStream,
          }),
        },
      }),
    );
    setContainer({
      client: () =>
        ({
          sessions: { snapshot: readSnapshot },
          runs: { subscribe },
        }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => expect(releaseOldNext).toBeTypeOf("function"));
    expect(selectCurrentRootRun(useAgentStore.getState().sessions[RID]!.view)?.id).toBe(
      "run_before_restart",
    );

    restarted = true;
    const synchronization = useAgentStore.getState().sessions[RID]!.synchronize!("replace-live");
    await expect(synchronization).resolves.toBe(true);

    const view = useAgentStore.getState().sessions[RID]!.view;
    expect(closeOldStream).toHaveBeenCalledOnce();
    expect(selectCurrentRootRun(view)).toMatchObject({
      id: "run_after_restart",
      status: "waiting",
    });
    expect(view.shared.plan).toMatchObject({ revision: 2 });
    expect(view.pendingInterrupts).toHaveLength(1);
    expect(view.toolCalls.item_after_restart).toMatchObject({
      name: "shell",
      status: "requires-action",
    });
    expect(readSnapshot).toHaveBeenCalledTimes(2);

    releaseOldNext({ value: undefined as never, done: true });
  });

  it("replaces a non-cooperative durable snapshot before folding the restarted Runtime", async () => {
    let restarted = false;
    let firstSignal: AbortSignal | undefined;
    let releaseOldSnapshot!: (snapshot: ReturnType<typeof materialSnapshot>) => void;
    const oldSnapshot = new Promise<ReturnType<typeof materialSnapshot>>((resolve) => {
      releaseOldSnapshot = resolve;
    });
    const readSnapshot = vi.fn(
      (_sessionId: unknown, _includeDescendants: unknown, signal?: AbortSignal) => {
        if (!restarted) {
          firstSignal = signal;
          return oldSnapshot;
        }
        return Promise.resolve(
          materialSnapshot({
            items: [
              {
                id: "item_restarted_tool",
                runId: "run_restarted_waiting",
                type: "toolCall",
                status: "running",
                startedAt: "2026-08-13T00:00:01.000Z",
                tool: { name: "shell", arguments: { command: "npm test" } },
              },
            ],
            runs: [
              runRef({
                id: "run_restarted_waiting",
                sessionId: RID,
                status: "waiting",
                activeSegmentId: undefined,
              }),
            ],
            interrupts: [
              {
                rootRunId: "run_restarted_waiting",
                sessionId: RID,
                createdAt: "2026-08-13T00:00:02.000Z",
                interrupts: [
                  {
                    type: "approval",
                    itemId: "item_restarted_tool",
                    runId: "run_restarted_waiting",
                    payload: {
                      tool: { name: "shell", arguments: { command: "npm test" } },
                    },
                  },
                ],
              },
            ],
            state: {
              type: "plan",
              sessionId: RID,
              revision: 2,
              plan: [
                {
                  id: "step_restarted",
                  description: "Approve recovered tool",
                  status: "in_progress",
                },
              ],
            },
          }),
        );
      },
    );
    setContainer({
      client: () =>
        ({
          sessions: { snapshot: readSnapshot },
        }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => expect(firstSignal).toBeInstanceOf(AbortSignal));
    restarted = true;
    const synchronization = useAgentStore.getState().sessions[RID]!.synchronize!("replace-live");
    await expect(synchronization).resolves.toBe(true);

    expect(firstSignal?.aborted).toBe(true);
    const view = useAgentStore.getState().sessions[RID]!.view;
    expect(selectCurrentRootRun(view)).toMatchObject({
      id: "run_restarted_waiting",
      status: "waiting",
    });
    expect(view.shared.plan).toMatchObject({ revision: 2 });
    expect(view.pendingInterrupts).toHaveLength(1);
    expect(view.toolCalls.item_restarted_tool).toMatchObject({
      name: "shell",
      status: "requires-action",
    });

    releaseOldSnapshot(
      materialSnapshot({
        items: [
          {
            id: "item_from_retired_runtime",
            runId: "run_retired",
            status: "completed",
            createdAt: "2026-08-13T00:00:00.000Z",
            type: "agentMessage",
            content: [{ type: "text", text: "must stay retired" }],
          },
        ],
      }),
    );
    await Promise.resolve();
    expect(JSON.stringify(useAgentStore.getState().sessions[RID]!.view)).not.toContain(
      "must stay retired",
    );
  });

  it("closes a reattach stream whose opening resolves after Runtime replacement", async () => {
    let restarted = false;
    let resolveOldOpening!: (stream: {
      result: { runId: string; segmentId: string };
      events: AsyncIterable<RunEvent>;
    }) => void;
    const oldOpening = new Promise<{
      result: { runId: string; segmentId: string };
      events: AsyncIterable<RunEvent>;
    }>((resolve) => {
      resolveOldOpening = resolve;
    });
    const closeOldStream = vi.fn(async () => ({ value: undefined, done: true }) as const);
    const subscribe = vi.fn(() => oldOpening);
    const readSnapshot = vi.fn().mockImplementation(() =>
      Promise.resolve(
        materialSnapshot({
          runs: restarted
            ? []
            : [
                runRef({
                  id: "run_retired_opening",
                  sessionId: RID,
                  status: "running",
                  activeSegmentId: "seg_retired_opening",
                }),
              ],
          state: {
            type: "plan",
            sessionId: RID,
            revision: restarted ? 2 : 1,
            plan: [],
          },
        }),
      ),
    );
    setContainer({
      client: () =>
        ({
          sessions: { snapshot: readSnapshot },
          runs: { subscribe },
        }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => expect(subscribe).toHaveBeenCalledOnce());
    restarted = true;
    await expect(
      useAgentStore.getState().sessions[RID]!.synchronize!("replace-live"),
    ).resolves.toBe(true);
    expect(useAgentStore.getState().sessions[RID]!.view.shared.plan).toMatchObject({
      revision: 2,
    });

    resolveOldOpening({
      result: { runId: "run_retired_opening", segmentId: "seg_retired_opening" },
      events: {
        [Symbol.asyncIterator]: () =>
          ({
            next: vi.fn(),
            return: closeOldStream,
          }) as AsyncIterator<RunEvent>,
      },
    });
    await waitFor(() => expect(closeOldStream).toHaveBeenCalledOnce());
    expect(selectCurrentRootRun(useAgentStore.getState().sessions[RID]!.view)).toBeNull();
  });

  it("replaces a non-cooperative reconnect opening after the live stream drops", async () => {
    let restarted = false;
    let resolveReconnect!: (stream: {
      result: { runId: string; segmentId: string };
      events: AsyncIterable<RunEvent>;
    }) => void;
    const reconnectOpening = new Promise<{
      result: { runId: string; segmentId: string };
      events: AsyncIterable<RunEvent>;
    }>((resolve) => {
      resolveReconnect = resolve;
    });
    const closeReconnect = vi.fn(async () => ({ value: undefined, done: true }) as const);
    let subscribeCalls = 0;
    const subscribe = vi.fn(() => {
      subscribeCalls += 1;
      if (subscribeCalls === 1) {
        return Promise.resolve({
          result: { runId: "run_reconnect", segmentId: "seg_reconnect" },
          events: {
            [Symbol.asyncIterator]: () =>
              ({
                next: vi.fn(async () => ({ value: undefined, done: true }) as const),
              }) as AsyncIterator<RunEvent>,
          },
        });
      }
      return reconnectOpening;
    });
    const readSnapshot = vi.fn().mockImplementation(() =>
      Promise.resolve(
        materialSnapshot({
          runs: restarted
            ? []
            : [
                runRef({
                  id: "run_reconnect",
                  sessionId: RID,
                  status: "running",
                  activeSegmentId: "seg_reconnect",
                }),
              ],
          state: {
            type: "plan",
            sessionId: RID,
            revision: restarted ? 2 : 1,
            plan: [],
          },
        }),
      ),
    );
    setContainer({
      client: () =>
        ({
          sessions: { snapshot: readSnapshot },
          runs: { subscribe },
        }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => expect(subscribe).toHaveBeenCalledTimes(2));
    restarted = true;
    await expect(
      useAgentStore.getState().sessions[RID]!.synchronize!("replace-live"),
    ).resolves.toBe(true);

    const view = useAgentStore.getState().sessions[RID]!.view;
    expect(view.shared.plan).toMatchObject({ revision: 2 });
    expect(selectCurrentRootRun(view)).toBeNull();

    resolveReconnect({
      result: { runId: "run_reconnect", segmentId: "seg_reconnect" },
      events: {
        [Symbol.asyncIterator]: () =>
          ({
            next: vi.fn(),
            return: closeReconnect,
          }) as AsyncIterator<RunEvent>,
      },
    });
    await waitFor(() => expect(closeReconnect).toHaveBeenCalledOnce());
    expect(selectCurrentRootRun(useAgentStore.getState().sessions[RID]!.view)).toBeNull();
  });

  it("replaces a non-cooperative exact Run read after the live stream finishes", async () => {
    let restarted = false;
    let resolveExactRead!: (run: RunRef) => void;
    const exactRead = new Promise<RunRef>((resolve) => {
      resolveExactRead = resolve;
    });
    const get = vi.fn(() => exactRead);
    const subscribe = vi.fn(() =>
      Promise.resolve({
        result: { runId: "run_exact_read", segmentId: "seg_exact_read" },
        events: (async function* () {
          yield {
            eventId: "evt_exact_terminal",
            runId: "run_exact_read",
            segmentId: "seg_exact_read",
            timestamp: "2026-08-13T00:00:01.000Z",
            event: {
              type: "segment.finished",
              outcome: { type: "completed" },
              metrics: { steps: 1, activeDurationMillis: 1 },
            },
          } as RunEvent;
        })(),
      }),
    );
    const readSnapshot = vi.fn().mockImplementation(() =>
      Promise.resolve(
        materialSnapshot({
          runs: restarted
            ? []
            : [
                runRef({
                  id: "run_exact_read",
                  sessionId: RID,
                  status: "running",
                  activeSegmentId: "seg_exact_read",
                }),
              ],
          state: {
            type: "plan",
            sessionId: RID,
            revision: restarted ? 2 : 1,
            plan: [],
          },
        }),
      ),
    );
    setContainer({
      client: () =>
        ({
          sessions: { snapshot: readSnapshot },
          runs: { get, subscribe },
        }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => expect(get).toHaveBeenCalledOnce());
    restarted = true;
    await expect(
      useAgentStore.getState().sessions[RID]!.synchronize!("replace-live"),
    ).resolves.toBe(true);

    const view = useAgentStore.getState().sessions[RID]!.view;
    expect(view.shared.plan).toMatchObject({ revision: 2 });
    expect(selectCurrentRootRun(view)).toBeNull();
    expect(JSON.stringify(view)).not.toContain("run_exact_read");

    resolveExactRead(
      runRef({
        id: "run_exact_read",
        sessionId: RID,
        status: "finished",
        activeSegmentId: undefined,
        outcome: { type: "completed" },
        finishedAt: "2026-08-13T00:00:01.000Z",
      }),
    );
    await Promise.resolve();
    expect(JSON.stringify(useAgentStore.getState().sessions[RID]!.view)).not.toContain(
      "run_exact_read",
    );
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
    const readSnapshot = vi
      .fn()
      .mockResolvedValueOnce(materialSnapshot({ runs: [running] }))
      .mockResolvedValue(materialSnapshot({ runs: [finished] }));
    setContainer({
      client: () =>
        ({
          sessions: { snapshot: readSnapshot },
          runs: { subscribe },
        }) as unknown as LyraClient,
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => {
      expect(
        useAgentStore.getState().sessions[RID]!.view.runsById.run_raced_terminal,
      ).toMatchObject({ status: "finished", outcome: { type: "completed" } });
    });

    expect(readSnapshot).toHaveBeenCalledTimes(2);
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
    const readSnapshot = vi.fn().mockImplementation(() =>
      Promise.resolve(
        materialSnapshot({
          runs: canceled ? [rootAfter, childAfter] : [rootBefore, childBefore],
        }),
      ),
    );
    setContainer({
      client: () =>
        ({
          sessions: { snapshot: readSnapshot },
          runs: {
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
