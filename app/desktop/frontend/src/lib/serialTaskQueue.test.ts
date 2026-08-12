import { describe, expect, it, vi } from "vitest";
import { createKeyedSerialTaskQueue, createSerialTaskQueue } from "./serialTaskQueue";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
}

describe("serial task queue", () => {
  it("runs tasks in submission order", async () => {
    const first = deferred<string>();
    const second = vi.fn().mockResolvedValue("second");
    const queue = createSerialTaskQueue();

    const firstResult = queue.run(() => first.promise);
    const secondResult = queue.run(second);
    await Promise.resolve();
    expect(second).not.toHaveBeenCalled();

    first.resolve("first");
    await expect(firstResult).resolves.toBe("first");
    await expect(secondResult).resolves.toBe("second");
  });

  it("continues after a task fails", async () => {
    const first = deferred<string>();
    const second = vi.fn().mockResolvedValue("second");
    const queue = createSerialTaskQueue();

    const firstResult = queue.run(() => first.promise);
    const secondResult = queue.run(second);
    first.reject(new Error("failed"));

    await expect(firstResult).rejects.toThrow("failed");
    await expect(secondResult).resolves.toBe("second");
  });

  it("serializes equal keys without blocking independent keys", async () => {
    const firstA = deferred<string>();
    const secondA = vi.fn().mockResolvedValue("second-a");
    const firstB = vi.fn().mockResolvedValue("first-b");
    const queue = createKeyedSerialTaskQueue<string>();

    const firstAResult = queue.run("a", () => firstA.promise);
    const secondAResult = queue.run("a", secondA);
    const firstBResult = queue.run("b", firstB);
    await Promise.resolve();
    expect(secondA).not.toHaveBeenCalled();
    expect(firstB).toHaveBeenCalledTimes(1);

    firstA.resolve("first-a");
    await expect(firstAResult).resolves.toBe("first-a");
    await expect(secondAResult).resolves.toBe("second-a");
    await expect(firstBResult).resolves.toBe("first-b");
  });
});
