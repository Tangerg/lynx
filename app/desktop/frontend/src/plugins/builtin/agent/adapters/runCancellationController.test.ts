import { describe, expect, it, vi } from "vitest";
import { createRunCancellationController } from "./runCancellationController";

describe("run cancellation controller", () => {
  it("does not commit a delayed response over a newer material revision", async () => {
    const response = { type: "root" };
    const execute = vi.fn().mockResolvedValue(response);
    const commitIfCurrent = vi.fn().mockReturnValue(false);
    const onSettled = vi.fn();
    const controller = createRunCancellationController({
      isCancelled: () => false,
      markInteracted: vi.fn(),
      readTarget: () => ({ terminal: false, viewRevision: 7 }),
      execute,
      commitIfCurrent,
      revalidateTerminal: vi.fn(),
      onSettled,
      onFailure: vi.fn(),
    });

    controller.cancel("run_1");

    await vi.waitFor(() => expect(onSettled).toHaveBeenCalledOnce());
    expect(commitIfCurrent).toHaveBeenCalledWith(response, 7);
  });

  it("suppresses a command failure when authoritative projection is terminal", async () => {
    const commandFailure = new Error("another client won");
    const onFailure = vi.fn();
    const onSettled = vi.fn();
    const controller = createRunCancellationController({
      isCancelled: () => false,
      markInteracted: vi.fn(),
      readTarget: () => ({ terminal: false, viewRevision: 2 }),
      execute: vi.fn().mockRejectedValue(commandFailure),
      commitIfCurrent: vi.fn(),
      revalidateTerminal: vi.fn().mockResolvedValue(true),
      onSettled,
      onFailure,
    });

    controller.cancel("run_1");

    await vi.waitFor(() => expect(onSettled).toHaveBeenCalledOnce());
    expect(onFailure).not.toHaveBeenCalled();
  });

  it("preserves the command failure when revalidation fails", async () => {
    const commandFailure = new Error("cancel failed");
    const onFailure = vi.fn();
    const controller = createRunCancellationController({
      isCancelled: () => false,
      markInteracted: vi.fn(),
      readTarget: () => ({ terminal: false, viewRevision: 2 }),
      execute: vi.fn().mockRejectedValue(commandFailure),
      commitIfCurrent: vi.fn(),
      revalidateTerminal: vi.fn().mockRejectedValue(new Error("offline")),
      onSettled: vi.fn(),
      onFailure,
    });

    controller.cancel("run_1");

    await vi.waitFor(() => expect(onFailure).toHaveBeenCalledWith("run_1", commandFailure));
  });

  it("admits only one in-flight cancellation per Run", async () => {
    let resolve!: () => void;
    const execute = vi.fn(
      () =>
        new Promise<void>((settle) => {
          resolve = settle;
        }),
    );
    const controller = createRunCancellationController({
      isCancelled: () => false,
      markInteracted: vi.fn(),
      readTarget: () => ({ terminal: false, viewRevision: 2 }),
      execute,
      commitIfCurrent: vi.fn().mockReturnValue(true),
      revalidateTerminal: vi.fn(),
      onSettled: vi.fn(),
      onFailure: vi.fn(),
    });

    controller.cancel("run_1");
    controller.cancel("run_1");
    expect(execute).toHaveBeenCalledOnce();

    resolve();
    await vi.waitFor(() => expect(execute).toHaveBeenCalledOnce());
  });
});
