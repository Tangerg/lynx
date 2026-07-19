import { describe, expect, it, vi } from "vitest";
import { createWorkspaceEventLoop } from "./workspaceEventLoop";

describe("workspace event loop", () => {
  it("invalidates all caches when a lossy stream has a sequence gap", async () => {
    const controller = new AbortController();
    const invalidateAll = vi.fn();
    const handled: number[] = [];
    let receivedBoth!: () => void;
    const done = new Promise<void>((resolve) => {
      receivedBoth = resolve;
    });

    const loop = createWorkspaceEventLoop({
      async subscribe({ signal }) {
        return (async function* () {
          yield { type: "skills.changed", sequence: 41 };
          yield { type: "mcp.serverChanged", sequence: 43 };
          await new Promise<void>((resolve) => {
            signal.addEventListener("abort", () => resolve(), { once: true });
          });
        })();
      },
      handleEvent(event) {
        handled.push(event.sequence);
        if (handled.length === 2) receivedBoth();
      },
      invalidateAll,
      reportError: vi.fn(),
    });

    loop.start(controller.signal);
    await done;
    controller.abort();

    expect(handled).toEqual([41, 43]);
    expect(invalidateAll).toHaveBeenCalledTimes(2); // subscribe + detected gap
  });
});
