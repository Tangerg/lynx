import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { configureAgentRuntimeGateway, type AgentRuntimeGateway } from "./ports/runtimeGateway";
import { setApprovalMode } from "./approvalPolicy";
import { APPROVAL_MODE_KEY } from "./approvalPolicyQueries";
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
});
