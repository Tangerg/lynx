import { afterEach, describe, expect, it, vi } from "vitest";
import { browserFileTransfer } from "./browserFileTransfer";

afterEach(() => vi.restoreAllMocks());

describe("browserFileTransfer", () => {
  it("settles native picker cancellation as an empty selection", async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});

    const selection = browserFileTransfer().pickText("application/json,.json");
    const observed = observe(selection);
    const picker = click.mock.instances[0] as HTMLInputElement;
    expect(picker).toBeDefined();

    picker.dispatchEvent(new Event("cancel"));
    await drainMicrotasks();
    const settledAtCancel = observed.settled();

    // Releases the old implementation after preserving the failure fact.
    picker.dispatchEvent(new Event("change"));
    await expect(selection).resolves.toBeNull();
    expect(settledAtCancel).toBe(true);
    expect(observed.value()).toBeNull();
  });
});

function observe<T>(operation: Promise<T>): {
  settled(): boolean;
  value(): T | undefined;
} {
  let settled = false;
  let value: T | undefined;
  void operation.then((result) => {
    value = result;
    settled = true;
  });
  return {
    settled: () => settled,
    value: () => value,
  };
}

async function drainMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index++) await Promise.resolve();
}
