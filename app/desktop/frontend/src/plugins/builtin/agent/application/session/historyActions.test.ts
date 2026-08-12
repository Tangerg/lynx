import { afterEach, describe, expect, it, vi } from "vitest";
import { configureAgentRuntimeGateway, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { rollbackSessionToBeforeRun } from "./historyActions";

let restoreRuntime: (() => void) | undefined;

afterEach(() => {
  restoreRuntime?.();
  restoreRuntime = undefined;
});

describe("rollbackSessionToBeforeRun", () => {
  it("does not issue a rollback when the session is authoritatively absent", async () => {
    const rollbackSession = vi.fn();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn().mockResolvedValue(null),
      rollbackSession,
    } as unknown as AgentRuntimeGateway);

    await expect(rollbackSessionToBeforeRun("ses_gone", "run_gone")).resolves.toBe(false);
    expect(rollbackSession).not.toHaveBeenCalled();
  });
});
