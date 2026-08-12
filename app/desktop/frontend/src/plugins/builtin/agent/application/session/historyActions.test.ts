import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { RunRef } from "@/rpc";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import {
  configureAgentRuntimeGateway,
  type AgentRuntimeGateway,
  type AgentSessionSnapshot,
} from "../ports/runtimeGateway";
import { configureAgentSessionViewPort, type AgentSessionViewPort } from "../ports/sessionView";
import { rollbackSessionToBeforeRun } from "./historyActions";

let restoreRuntime: (() => void) | undefined;
let restoreView: (() => void) | undefined;

beforeAll(async () => {
  const { default: foldPlugin } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(foldPlugin);
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
      loadSessionSnapshot: vi.fn().mockResolvedValue(snapshot()),
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
    let release!: (value: AgentSessionSnapshot) => void;
    const firstRead = new Promise<AgentSessionSnapshot>((resolve) => {
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
    release(snapshot());
    await expect(first).resolves.toMatchObject({ status: "committed" });
    expect(rollbackSession).toHaveBeenCalledOnce();
  });

  it.each(["files", "both"] as const)(
    "does not degrade first-turn %s restore into destructive history rollback",
    async (restoreType) => {
      const rollbackSession = vi.fn();
      restoreRuntime = configureAgentRuntimeGateway({
        loadSessionSnapshot: vi.fn().mockResolvedValue({
          runs: [run("run_1", "2026-08-12T00:00:00.000Z")],
          items: [],
          pendingInterruptSets: [],
        }),
        rollbackSession,
      } as unknown as AgentRuntimeGateway);

      await expect(rollbackSessionToBeforeRun("ses_1", "run_1", restoreType)).resolves.toEqual({
        status: "unavailable",
      });
      expect(rollbackSession).not.toHaveBeenCalled();
    },
  );
});

function snapshot(): AgentSessionSnapshot {
  return {
    runs: [run("run_1", "2026-08-12T00:00:00.000Z"), run("run_2", "2026-08-12T00:01:00.000Z")],
    items: [],
    pendingInterruptSets: [],
  };
}

function run(id: string, createdAt: string): RunRef {
  return {
    id,
    sessionId: "ses_1",
    status: "finished",
    createdAt,
    finishedAt: createdAt,
    outcome: { type: "completed" },
    metrics: { steps: 1, activeDurationMillis: 1 },
    protocolProfile: { interruptTypes: [], requiredFeatures: [] },
  };
}
