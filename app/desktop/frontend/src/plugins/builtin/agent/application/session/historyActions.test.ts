import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { AgentRunFact as RunRef } from "@/plugins/sdk";
import { setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { navigator } from "@/lib/navigation";
import { installAgentRuntimeGateway } from "../../adapters/agentRuntimeGateway";
import {
  configureAgentRuntimeGateway,
  type AgentRuntimeGateway,
  type AgentSessionMaterialRead,
  type AgentSessionSnapshot,
} from "../ports/runtimeGateway";
import { configureAgentSessionViewPort, type AgentSessionViewPort } from "../ports/sessionView";
import { forkAgentSessionAtRun, rollbackSessionToBeforeRun } from "./historyActions";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

let restoreRuntime: (() => void) | undefined;
let restoreView: (() => void) | undefined;

beforeAll(async () => {
  const { default: foldPlugin } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(foldPlugin);
});

afterEach(() => {
  restoreRuntime?.();
  restoreView?.();
  restoreRuntime = undefined;
  restoreView = undefined;
});

describe("rollbackSessionToBeforeRun", () => {
  it("does not issue a rollback when the session is authoritatively absent", async () => {
    const rollbackSession = vi.fn();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn().mockResolvedValue(null),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);

    await expect(rollbackSessionToBeforeRun("ses_gone", "run_gone")).resolves.toEqual({
      status: "unavailable",
    });
    expect(rollbackSession).not.toHaveBeenCalled();
  });

  it("returns the Runtime-authored input dropped by the committed rollback", async () => {
    const synchronize = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
    restoreView = configureAgentSessionViewPort({
      getSession: () => ({ synchronize }),
    } as unknown as AgentSessionViewPort);
    const rollbackSession = vi.fn().mockResolvedValue({
      droppedRuns: [
        {
          runId: "run_2",
          userInput: {
            parts: [
              { kind: "text", text: "authoritative prompt" },
              { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
            ],
          },
        },
      ],
    });
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn().mockResolvedValue(material(snapshot())),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);

    await expect(rollbackSessionToBeforeRun("ses_1", "run_2")).resolves.toEqual({
      status: "committed",
      userInput: {
        parts: [
          { kind: "text", text: "authoritative prompt" },
          { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      },
    });
    expect(rollbackSession).toHaveBeenCalledWith({ sessionId: "ses_1", toRunId: "run_1" });
    expect(synchronize).toHaveBeenCalledTimes(2);
  });

  it("admits only one destructive history rewrite per Session", async () => {
    let release!: (value: AgentSessionMaterialRead) => void;
    const firstRead = new Promise<AgentSessionMaterialRead>((resolve) => {
      release = resolve;
    });
    const rollbackSession = vi.fn().mockResolvedValue({ droppedRuns: [] });
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => firstRead),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);
    restoreView = configureAgentSessionViewPort({
      getSession: () => ({ synchronize: vi.fn().mockResolvedValue(true) }),
    } as unknown as AgentSessionViewPort);

    const first = rollbackSessionToBeforeRun("ses_1", "run_2");
    await expect(rollbackSessionToBeforeRun("ses_1", "run_2")).resolves.toEqual({
      status: "inFlight",
    });
    release(material(snapshot()));
    await expect(first).resolves.toMatchObject({ status: "committed" });
    expect(rollbackSession).toHaveBeenCalledOnce();
  });

  it("does not attach an old snapshot inspection to the successor rollback writer", async () => {
    const read = deferred<AgentSessionMaterialRead>();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);
    restoreView = configureAgentSessionViewPort({
      getSession: () => ({ synchronize: vi.fn().mockResolvedValue(true) }),
    } as unknown as AgentSessionViewPort);
    const successorRollback = vi.fn().mockResolvedValue({ droppedRuns: [] });
    setContainer({
      client: () => ({ sessions: { rollback: successorRollback } }) as unknown as LyraClient,
    });

    const retired = rollbackSessionToBeforeRun("ses_1", "run_2");
    const disposeSuccessor = installAgentRuntimeGateway();
    let retiredSettled = false;
    void retired.then(() => {
      retiredSettled = true;
    });
    await flushMicrotasks();
    const retiredSettledBeforeOldRead = retiredSettled;
    read.resolve(material(snapshot()));
    try {
      await expect(retired).resolves.toEqual({ status: "unavailable" });
      expect(retiredSettledBeforeOldRead).toBe(true);
      expect(successorRollback).not.toHaveBeenCalled();
    } finally {
      disposeSuccessor.dispose();
    }
  });

  it.each(["files", "both"] as const)(
    "does not degrade first-turn %s restore into destructive history rollback",
    async (restoreType) => {
      const rollbackSession = vi.fn();
      restoreRuntime = configureAgentRuntimeGateway({
        loadSessionSnapshot: vi.fn().mockResolvedValue(
          material({
            runs: [run("run_1", "2026-08-12T00:00:00.000Z")],
            items: [],
            pendingInterruptSets: [],
          }),
        ),
        rollbackSession,
      } as unknown as AgentRuntimeGateway);

      await expect(rollbackSessionToBeforeRun("ses_1", "run_1", restoreType)).resolves.toEqual({
        status: "unavailable",
      });
      expect(rollbackSession).not.toHaveBeenCalled();
    },
  );
});

describe("forkAgentSessionAtRun", () => {
  it("does not join an old fork or let its response navigate the successor", async () => {
    const retiredFork = deferred<{ id: string }>();
    restoreRuntime = configureAgentRuntimeGateway({
      forkSession: vi.fn(() => retiredFork.promise),
    } as unknown as AgentRuntimeGateway);
    const successorFork = vi.fn().mockResolvedValue({ id: "fork_successor" });
    setContainer({
      client: () => ({ sessions: { fork: successorFork } }) as unknown as LyraClient,
    });

    const retired = forkAgentSessionAtRun("ses_1", "run_1");
    const disposeSuccessor = installAgentRuntimeGateway();
    const successor = forkAgentSessionAtRun("ses_1", "run_1");
    await Promise.resolve();
    const successorStartedBeforeRetiredSettlement = successorFork.mock.calls.length;
    let retiredSettled = false;
    void retired.then(() => {
      retiredSettled = true;
    });
    await flushMicrotasks();
    const retiredSettledBeforeOldRPC = retiredSettled;
    retiredFork.resolve({ id: "fork_retired" });
    try {
      await Promise.all([retired, successor]);
      expect(successorStartedBeforeRetiredSettlement).toBe(1);
      expect(retiredSettledBeforeOldRPC).toBe(true);
      expect(navigator().get().session).toBe("fork_successor");
    } finally {
      disposeSuccessor.dispose();
    }
  });
});

function snapshot(): AgentSessionSnapshot {
  return {
    runs: [run("run_1", "2026-08-12T00:00:00.000Z"), run("run_2", "2026-08-12T00:01:00.000Z")],
    items: [],
    pendingInterruptSets: [],
  };
}

function material(value: AgentSessionSnapshot): AgentSessionMaterialRead {
  return { snapshot: value, commitAssociatedReadModels: vi.fn() };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

async function flushMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
}

function run(id: string, createdAt: string): RunRef {
  return {
    id,
    sessionId: "ses_1",
    status: "finished",
    parentRunId: null,
    rootRunId: id,
    spawnedByItemId: null,
    activeSegmentId: null,
    createdAt,
    finishedAt: createdAt,
    outcome: { type: "completed" },
    metrics: {
      steps: 1,
      activeDurationMillis: 1,
      usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
    },
  };
}
