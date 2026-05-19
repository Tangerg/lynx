import { describe, expect, it, vi } from "vitest";
import { definePlugin, loadPlugin, loadPlugins } from "./definePlugin";
import { usePluginErrorStore } from "./errors";
import { usePluginStore } from "./registry";

describe("definePlugin", () => {
  it("is an identity function", () => {
    const spec = { name: "a", version: "1.0.0", setup: () => {} };
    expect(definePlugin(spec)).toBe(spec);
  });
});

describe("loadPlugin", () => {
  it("runs setup, records the plugin as loaded", async () => {
    const setup = vi.fn();
    const spec = definePlugin({ name: "a", version: "1.0.0", setup });

    const result = await loadPlugin(spec);

    expect(result).toEqual({ kind: "loaded", name: "a" });
    expect(setup).toHaveBeenCalledOnce();
    expect(usePluginStore.getState().loaded.has("a")).toBe(true);
  });

  it("disposes already-registered handles when setup throws later", async () => {
    const StubComponent = () => null;
    const result = await loadPlugin(definePlugin({
      name: "a",
      version: "1.0.0",
      setup: ({ host }) => {
        // Register first…
        host.tool.registerPreview("bash", StubComponent);
        // …then explode.
        throw new Error("kaboom");
      },
    }));

    expect(result.kind).toBe("failed");
    // The registry must be empty — the bash entry was rolled back.
    expect(usePluginStore.getState().toolPreviews.size).toBe(0);
    expect(usePluginStore.getState().loaded.has("a")).toBe(false);
    // And the error landed in the log under source="setup".
    const log = usePluginErrorStore.getState().log;
    expect(log).toHaveLength(1);
    expect(log[0].plugin).toBe("a");
    expect(log[0].source).toBe("setup");
    expect(log[0].message).toBe("kaboom");
  });

  it("rejects plugins whose apiVersion doesn't match the host", async () => {
    const setup = vi.fn();
    const result = await loadPlugin(definePlugin({
      name: "tooOld",
      version: "1.0.0",
      apiVersion: "^2.0.0",        // host is 1.0.0
      setup,
    }));

    expect(result).toMatchObject({ kind: "skipped", name: "tooOld" });
    expect(setup).not.toHaveBeenCalled();
    expect(usePluginErrorStore.getState().log[0].source).toBe("setup");
  });

  it("accepts plugins with a matching range", async () => {
    const result = await loadPlugin(definePlugin({
      name: "ok",
      version: "1.0.0",
      apiVersion: "^1.0.0",
      setup: () => {},
    }));
    expect(result.kind).toBe("loaded");
  });

  it("treats a missing apiVersion as implicit trust (loaded)", async () => {
    const result = await loadPlugin(definePlugin({
      name: "implicit",
      version: "1.0.0",
      setup: () => {},
    }));
    expect(result.kind).toBe("loaded");
  });

  it("rejects an invalid apiVersion string with a clear reason", async () => {
    const result = await loadPlugin(definePlugin({
      name: "garbage",
      version: "1.0.0",
      apiVersion: "not-a-range",
      setup: () => {},
    }));
    expect(result.kind).toBe("skipped");
    if (result.kind === "skipped") {
      expect(result.reason).toMatch(/invalid apiVersion/);
    }
  });
});

describe("loadPlugins", () => {
  it("loads in order, isolating failures per plugin", async () => {
    const order: string[] = [];
    const results = await loadPlugins([
      definePlugin({ name: "a", version: "1.0.0", setup: () => { order.push("a"); } }),
      definePlugin({ name: "b", version: "1.0.0", setup: () => { throw new Error("nope"); } }),
      definePlugin({ name: "c", version: "1.0.0", setup: () => { order.push("c"); } }),
    ]);

    expect(order).toEqual(["a", "c"]);
    expect(results.map((r) => r.kind)).toEqual(["loaded", "failed", "loaded"]);
    expect(usePluginStore.getState().loaded.has("a")).toBe(true);
    expect(usePluginStore.getState().loaded.has("b")).toBe(false);
    expect(usePluginStore.getState().loaded.has("c")).toBe(true);
  });
});
