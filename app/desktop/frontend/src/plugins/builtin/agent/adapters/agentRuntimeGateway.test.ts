import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import {
  RpcError,
  RpcTransportError,
  UNARY_MUTATION_ATTEMPT_TIMEOUT_MS,
  type LyraClient,
  type Methods,
  type MutationPromise,
  UnaryMutationSettlementClosedError,
} from "@/rpc";
import { asRunId, asSegmentId, asSessionId } from "@/rpc";
import { createMutationPromise } from "@/rpc/mutation";
import * as runtimeCapabilities from "@/plugins/builtin/runtime/public/capabilities";
import { agentRuntime } from "../application/ports/runtimeGateway";
import {
  installAgentRuntimeGateway,
  type AgentRuntimeGatewayInstallation,
} from "./agentRuntimeGateway";
import { registerAgentSessionSharedMaterial } from "../application/ports/sessionSharedMaterial";

let uninstall: AgentRuntimeGatewayInstallation | undefined;
let uninstallMaterialCommitter: (() => void) | undefined;

afterEach(() => {
  uninstall?.dispose();
  uninstall = undefined;
  uninstallMaterialCommitter?.();
  uninstallMaterialCommitter = undefined;
  resetContainer();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("agentRuntimeGateway", () => {
  it("does not hand a retained create promise to a replacement adapter generation", async () => {
    const transportFailure = new RpcTransportError("retired response was lost");
    const retiredRetry = vi.fn(
      () =>
        Object.assign(Promise.resolve({ id: asSessionId("ses_retired") }), {
          idempotencyKey: "retired-create",
          retry: vi.fn(),
        }) as MutationPromise<{ id: ReturnType<typeof asSessionId> }>,
    );
    const retiredCreate = vi.fn(
      () =>
        Object.assign(Promise.reject(transportFailure), {
          idempotencyKey: "retired-create",
          retry: retiredRetry,
        }) as MutationPromise<{ id: ReturnType<typeof asSessionId> }>,
    );
    setContainer({
      client: () => ({ sessions: { create: retiredCreate } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(agentRuntime().createSession({ cwd: "/repo" })).rejects.toBe(transportFailure);
    uninstall.dispose();
    uninstall = undefined;

    const successorCreate = vi.fn(
      () =>
        Object.assign(Promise.resolve({ id: asSessionId("ses_successor") }), {
          idempotencyKey: "successor-create",
          retry: vi.fn(),
        }) as MutationPromise<{ id: ReturnType<typeof asSessionId> }>,
    );
    setContainer({
      client: () => ({ sessions: { create: successorCreate } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(agentRuntime().createSession({ cwd: "/repo" })).resolves.toEqual({
      id: "ses_successor",
    });
    expect(successorCreate).toHaveBeenCalledOnce();
    expect(retiredRetry).not.toHaveBeenCalled();
  });

  it("refuses a create response that settles after its adapter generation is disposed", async () => {
    let settleRetired!: (value: { id: ReturnType<typeof asSessionId> }) => void;
    const retired = new Promise<{ id: ReturnType<typeof asSessionId> }>((resolve) => {
      settleRetired = resolve;
    });
    let attemptSignal: AbortSignal | undefined;
    const create = vi.fn((_params, signal?: AbortSignal) => {
      attemptSignal = signal;
      return Object.assign(retired, {
        idempotencyKey: "retired-create",
        retry: vi.fn(),
      }) as MutationPromise<{ id: ReturnType<typeof asSessionId> }>;
    });
    setContainer({
      client: () => ({ sessions: { create } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    const creating = agentRuntime().createSession({ cwd: "/repo" });
    uninstall.dispose();
    uninstall = undefined;

    await expect(creating).rejects.toBeInstanceOf(UnaryMutationSettlementClosedError);
    expect(attemptSignal?.aborted).toBe(true);

    settleRetired({ id: asSessionId("ses_retired") });
    await retired;
  });

  it("replays a timed-out create with the same mutation identity and a fresh signal", async () => {
    vi.useFakeTimers();
    const keys: string[] = [];
    const signals: AbortSignal[] = [];
    let executions = 0;
    const create = vi.fn((_params, signal?: AbortSignal) =>
      createMutationPromise(
        async (key, attempt) => {
          keys.push(key);
          signals.push(attempt.signal!);
          executions += 1;
          if (executions === 2) return { id: asSessionId("ses_replayed") };
          await new Promise<void>((_resolve, reject) => {
            attempt.signal?.addEventListener(
              "abort",
              () => reject(new RpcTransportError("attempt timed out")),
              { once: true },
            );
          });
          throw new Error("unreachable");
        },
        "logical-create",
        { signal },
      ),
    );
    setContainer({
      client: () => ({ sessions: { create } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    const creating = agentRuntime().createSession({ cwd: "/repo" });
    await vi.advanceTimersByTimeAsync(0);
    expect(executions).toBe(1);
    await vi.advanceTimersByTimeAsync(UNARY_MUTATION_ATTEMPT_TIMEOUT_MS);

    await expect(creating).resolves.toEqual({ id: "ses_replayed" });
    expect(create).toHaveBeenCalledOnce();
    expect(keys).toEqual(["logical-create", "logical-create"]);
    expect(signals).toHaveLength(2);
    expect(signals[0]).not.toBe(signals[1]);
    expect(signals[0]?.aborted).toBe(true);
    expect(signals[1]?.aborted).toBe(false);
  });

  it("forwards the caller snapshot revision without a get-before-write", async () => {
    const get = vi.fn();
    const update = vi.fn().mockResolvedValue({ revision: 12 });
    setContainer({
      client: () => ({ sessions: { get, update } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(
      agentRuntime().updateSession({
        sessionId: "ses_1",
        expectedRevision: 11,
        favorite: true,
      }),
    ).resolves.toEqual({ revision: 12 });

    expect(update).toHaveBeenCalledWith({
      sessionId: asSessionId("ses_1"),
      expectedRevision: 11,
      favorite: true,
    } satisfies Parameters<Methods["sessions"]["update"]>[0]);
    expect(get).not.toHaveBeenCalled();
  });

  it("projects the approval mode saved by the Runtime", async () => {
    const setMode = vi.fn().mockResolvedValue({ mode: "safe" });
    setContainer({
      client: () => ({ approval: { setMode } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(agentRuntime().setApprovalMode("safe")).resolves.toBe("safe");
    expect(setMode).toHaveBeenCalledWith("safe");
  });

  it("translates structured steering input only at the runtime adapter", async () => {
    const steer = vi.fn().mockResolvedValue({});
    setContainer({
      client: () => ({ runs: { steer } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await agentRuntime().steerRun("run_1", "seg_1", {
      parts: [
        { kind: "text", text: "compare this" },
        { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
      ],
    });

    expect(steer).toHaveBeenCalledWith(asRunId("run_1"), asSegmentId("seg_1"), [
      { type: "text", text: "compare this" },
      { type: "image", mime: "image/png", data: "aW1hZ2U=" },
    ] satisfies Parameters<Methods["runs"]["steer"]>[2]);
  });

  it.each([{ supported: false }, { supported: true }])(
    "reads one coherent snapshot with descendants supported=$supported",
    async ({ supported }) => {
      vi.spyOn(runtimeCapabilities, "runtimeCapability").mockReturnValue(supported);
      const stageMaterial = vi.fn(
        (_sessionId: string, material: { goal?: { objective: string } }) =>
          material.goal?.objective,
      );
      uninstallMaterialCommitter = registerAgentSessionSharedMaterial(
        "test.goal-objective",
        stageMaterial,
      );
      const readSnapshot = vi.fn().mockResolvedValue({
        items: [],
        runs: [],
        interrupts: [],
        state: {
          type: "plan",
          sessionId: "ses_1",
          revision: 4,
          plan: [{ id: "step_1", description: "Verify boundaries", status: "in_progress" }],
        },
        goal: {
          sessionId: "ses_1",
          objective: "Recover every mounted read",
          status: "active",
          budget: {},
          used: { runs: 1, costUsd: 0.25, steps: 2 },
          createdAt: "2026-08-17T00:00:00Z",
          updatedAt: "2026-08-17T00:01:00Z",
        },
      });
      setContainer({
        client: () =>
          ({
            sessions: { snapshot: readSnapshot },
          }) as unknown as LyraClient,
      });
      uninstall = installAgentRuntimeGateway();

      const snapshot = await agentRuntime().loadSessionSnapshot("ses_1");

      expect(readSnapshot).toHaveBeenCalledWith(asSessionId("ses_1"), supported, undefined);
      expect(snapshot?.snapshot.state).toEqual({
        type: "plan",
        revision: 4,
        plan: [{ id: "step_1", text: "Verify boundaries", status: "active" }],
      });
      expect(stageMaterial).toHaveBeenCalledWith(
        "ses_1",
        expect.objectContaining({
          goal: expect.objectContaining({ objective: "Recover every mounted read" }),
        }),
      );
      expect(snapshot?.projectAssociatedSharedMaterial({ plan: "kept" })).toEqual({
        plan: "kept",
        "test.goal-objective": "Recover every mounted read",
      });
    },
  );

  it("translates an authoritatively missing session into an absent snapshot", async () => {
    const missing = new RpcError({
      code: -32002,
      message: "session missing",
      data: { type: "session_not_found" },
    });
    setContainer({
      client: () =>
        ({
          sessions: { snapshot: vi.fn().mockRejectedValue(missing) },
        }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(agentRuntime().loadSessionSnapshot("ses_gone")).resolves.toBeNull();
  });

  it("treats an already missing Session as a completed delete", async () => {
    const missing = new RpcError({
      code: -32002,
      message: "session missing",
      data: { type: "session_not_found" },
    });
    setContainer({
      client: () =>
        ({ sessions: { delete: vi.fn().mockRejectedValue(missing) } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(agentRuntime().deleteSession("ses_gone")).resolves.toBeUndefined();
  });

  it("projects rollback dropped input without leaking wire blocks into Application", async () => {
    const rollback = vi.fn().mockResolvedValue({
      session: {},
      droppedRuns: [
        {
          run: { id: "run_dropped", sessionId: "ses_1" },
          userInput: [
            { type: "text", text: "retry this" },
            { type: "image", mime: "image/png", data: "aW1hZ2U=" },
          ],
        },
      ],
    });
    setContainer({
      client: () => ({ sessions: { rollback } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(
      agentRuntime().rollbackSession({
        sessionId: "ses_1",
        toRunId: "run_keep",
        restoreType: "both",
      }),
    ).resolves.toEqual({
      droppedRuns: [
        {
          runId: "run_dropped",
          userInput: {
            parts: [
              { kind: "text", text: "retry this" },
              { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
            ],
          },
        },
      ],
    });
    expect(rollback).toHaveBeenCalledWith({
      sessionId: asSessionId("ses_1"),
      toRunId: asRunId("run_keep"),
      restoreType: "both",
    });
  });
});
