import { describe, expect, it, vi } from "vitest";
import { BEFORE_UNLOAD_HANDLER, definePlugin } from "@/plugins/sdk";
import { configureHostTeardown, publishHostBridge } from "./hostBridge";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

describe("host bridge teardown", () => {
  it("runs plugin unload handlers before the composition root teardown", async () => {
    const order: string[] = [];
    await loadPluginsForTest(
      definePlugin({
        name: "test.host-before-unload",
        setup(ctx) {
          ctx.contribute(BEFORE_UNLOAD_HANDLER, () => order.push("plugin"));
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
