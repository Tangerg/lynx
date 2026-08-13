import { describe, expect, it, vi } from "vitest";
import { definePlugin, loadPlugin } from "@/plugins/sdk";
import { configureHostTeardown, publishHostBridge } from "./hostBridge";

describe("host bridge teardown", () => {
  it("runs plugin unload handlers before the composition root teardown", async () => {
    const order: string[] = [];
    await loadPlugin(
      definePlugin({
        name: "test.host-before-unload",
        version: "1.0.0",
        setup({ host }) {
          host.lifecycle.onBeforeUnload(() => order.push("plugin"));
        },
      }),
    );
    const teardown = vi.fn(() => order.push("host"));
    const release = configureHostTeardown(teardown);
    publishHostBridge();

    window.dispatchEvent(new Event("beforeunload"));

    expect(order).toEqual(["plugin", "host"]);
    expect(teardown).toHaveBeenCalledOnce();
    release();
  });
});
