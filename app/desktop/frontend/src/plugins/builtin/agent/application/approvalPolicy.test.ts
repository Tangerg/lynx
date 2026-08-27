import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { ScopeAppClient } from "@/rpc";
import { installAgentRuntimeGateway } from "../adapters/agentRuntimeGateway";
import { configureAgentRuntimeGateway, type AgentRuntimeGateway } from "./ports/runtimeGateway";
import { forgetRules, setApprovalMode } from "./approvalPolicy";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
  type ApprovalRuleSummary,
} from "./approvalPolicyQueries";
import type { ApprovalMode } from "../domain/hitl";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
}

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  queryClient.removeQueries({ queryKey: [APPROVAL_MODE_KEY] });
  queryClient.removeQueries({ queryKey: [APPROVAL_RULES_KEY] });
  vi.restoreAllMocks();
  resetContainer();
});

describe("approval policy", () => {
  it("serializes mode changes and commits each authoritative response", async () => {
    const first = deferred<ApprovalMode>();
    const second = deferred<ApprovalMode>();
    const setMode = vi
      .fn<(mode: ApprovalMode) => Promise<ApprovalMode>>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    uninstall = configureAgentRuntimeGateway({
      setApprovalMode: setMode,
    } as unknown as AgentRuntimeGateway);
    queryClient.setQueryData([APPROVAL_MODE_KEY], "balanced");

    const safe = setApprovalMode("safe");
    const yolo = setApprovalMode("yolo");
    await Promise.resolve();
    expect(setMode).toHaveBeenCalledTimes(1);

    first.resolve("safe");
    await expect(safe).resolves.toBe("safe");
    await Promise.resolve();
    expect(setMode).toHaveBeenNthCalledWith(2, "yolo");

    second.resolve("yolo");
    await expect(yolo).resolves.toBe("yolo");
    expect(queryClient.getQueryData([APPROVAL_MODE_KEY])).toBe("yolo");
  });

  it("continues with the next mode after a rejected change", async () => {
    const first = deferred<ApprovalMode>();
    const setMode = vi
      .fn<(mode: ApprovalMode) => Promise<ApprovalMode>>()
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce("yolo");
    uninstall = configureAgentRuntimeGateway({
      setApprovalMode: setMode,
    } as unknown as AgentRuntimeGateway);

    const rejected = setApprovalMode("safe");
    const accepted = setApprovalMode("yolo");
    first.reject(new Error("not saved"));

    await expect(rejected).rejects.toThrow("not saved");
    await expect(accepted).resolves.toBe("yolo");
    expect(setMode).toHaveBeenNthCalledWith(2, "yolo");
  });

  it("does not serialize the successor behind an old Plugin Host mode change", async () => {
    const retiredMode = deferred<ApprovalMode>();
    uninstall = configureAgentRuntimeGateway({
      setApprovalMode: vi.fn(() => retiredMode.promise),
    } as unknown as AgentRuntimeGateway);
    const retired = setApprovalMode("safe");

    const successorSetMode = vi.fn().mockResolvedValue({ mode: "yolo" });
    setContainer({
      client: () => ({ approval: { setMode: successorSetMode } }) as unknown as ScopeAppClient,
    });
    const disposeSuccessor = installAgentRuntimeGateway();
    const successor = setApprovalMode("yolo");
    await Promise.resolve();
    const successorStartedBeforeRetiredSettlement = successorSetMode.mock.calls.length;
    retiredMode.resolve("safe");
    try {
      await Promise.allSettled([retired, successor]);
      expect(successorStartedBeforeRetiredSettlement).toBe(1);
      expect(successorSetMode).toHaveBeenCalledTimes(1);
      expect(successorSetMode).toHaveBeenCalledWith("yolo");
      expect(queryClient.getQueryData([APPROVAL_MODE_KEY])).toBe("yolo");
    } finally {
      disposeSuccessor.dispose();
    }
  });

  it("keeps accepted rule deletions when a later item in the batch fails", async () => {
    const failure = new Error("second delete failed");
    const forgetApprovalRule = vi
      .fn()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(failure);
    uninstall = configureAgentRuntimeGateway({
      forgetApprovalRule,
    } as unknown as AgentRuntimeGateway);
    const key = [APPROVAL_RULES_KEY, { sessionId: "ses_1" }];
    queryClient.setQueryData(key, [rule("rule-1"), rule("rule-2")]);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();

    await expect(forgetRules(["rule-1", "rule-2"])).rejects.toBe(failure);
    expect(queryClient.getQueryData(key)).toEqual([rule("rule-2")]);
  });

  it("keeps an accepted rule deletion successful when cache repair fails", async () => {
    uninstall = configureAgentRuntimeGateway({
      forgetApprovalRule: vi.fn().mockResolvedValue(undefined),
    } as unknown as AgentRuntimeGateway);
    const key = [APPROVAL_RULES_KEY, { sessionId: "ses_1" }];
    queryClient.setQueryData(key, [rule("rule-1")]);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("cache unavailable"));

    await expect(forgetRules(["rule-1"])).resolves.toBeUndefined();
    expect(queryClient.getQueryData(key)).toEqual([]);
  });

  it("does not continue a rule batch through a successor Plugin Host", async () => {
    const retiredWrite = deferred<void>();
    const forgetRetired = vi.fn(() => retiredWrite.promise);
    const forgetSuccessor = vi.fn().mockResolvedValue(undefined);
    setContainer({
      client: () => ({ approval: { forgetRule: forgetRetired } }) as unknown as ScopeAppClient,
    });
    const retiredInstallation = installAgentRuntimeGateway();
    const command = rejected(forgetRules(["rule-1", "rule-2"]));
    await vi.waitFor(() => expect(forgetRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ approval: { forgetRule: forgetSuccessor } }) as unknown as ScopeAppClient,
    });
    const successorInstallation = installAgentRuntimeGateway();
    try {
      await expect(command).resolves.toMatchObject({ message: "agent_command_owner_retired" });
      expect(forgetSuccessor).not.toHaveBeenCalled();
    } finally {
      retiredWrite.resolve();
      successorInstallation.dispose();
      retiredInstallation.dispose();
    }
  });
});

function rule(id: string): ApprovalRuleSummary {
  return {
    id,
    scope: "global",
    tool: "shell",
    decision: "allow",
  };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
