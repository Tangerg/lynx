import { describe, expect, it, vi } from "vitest";
import { definePlugin, loadPlugin, loadPlugins, reloadPlugin, unloadPlugin } from "./definePlugin";
import { usePluginErrorStore } from "./errors";
import { TOOL_PREVIEW } from "./kernelPoints";
import { usePluginStore } from "./registry";
import { lookupCommand, lookupExtensionPoint } from "./selectors";

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
    const result = await loadPlugin(
      definePlugin({
        name: "a",
        version: "1.0.0",
        setup: ({ host }) => {
          // Register first…
          host.tool.registerPreview("bash", StubComponent);
          // …then explode.
          throw new Error("kaboom");
        },
      }),
    );

    expect(result.kind).toBe("failed");
    // The registry must be empty — the bash entry was rolled back.
    expect(lookupExtensionPoint(TOOL_PREVIEW).length).toBe(0);
    expect(usePluginStore.getState().loaded.has("a")).toBe(false);
    // And the error landed in the log under source="setup".
    const log = usePluginErrorStore.getState().log;
    expect(log).toHaveLength(1);
    expect(log[0]!.plugin).toBe("a");
    expect(log[0]!.source).toBe("setup");
    expect(log[0]!.message).toBe("kaboom");
  });

  it("rejects plugins whose apiVersion doesn't match the host", async () => {
    const setup = vi.fn();
    const result = await loadPlugin(
      definePlugin({
        name: "tooOld",
        version: "1.0.0",
        apiVersion: "^2.0.0", // host is 1.0.0
        setup,
      }),
    );

    expect(result).toMatchObject({ kind: "skipped", name: "tooOld" });
    expect(setup).not.toHaveBeenCalled();
    expect(usePluginErrorStore.getState().log[0]!.source).toBe("setup");
  });

  it("accepts plugins with a matching range", async () => {
    const result = await loadPlugin(
      definePlugin({
        name: "ok",
        version: "1.0.0",
        apiVersion: "^1.0.0",
        setup: () => {},
      }),
    );
    expect(result.kind).toBe("loaded");
  });

  it("treats a missing apiVersion as implicit trust (loaded)", async () => {
    const result = await loadPlugin(
      definePlugin({
        name: "implicit",
        version: "1.0.0",
        setup: () => {},
      }),
    );
    expect(result.kind).toBe("loaded");
  });

  it("rejects an invalid apiVersion string with a clear reason", async () => {
    const result = await loadPlugin(
      definePlugin({
        name: "garbage",
        version: "1.0.0",
        apiVersion: "not-a-range",
        setup: () => {},
      }),
    );
    expect(result.kind).toBe("skipped");
    // Narrow via assertion before unconditional expect — keeps the
    // reason assertion non-conditional for vitest/no-conditional-expect.
    if (result.kind !== "skipped") throw new Error("unreachable");
    expect(result.reason).toMatch(/invalid apiVersion/);
  });
});

describe("loadPlugins", () => {
  it("loads in order, isolating failures per plugin", async () => {
    const order: string[] = [];
    const results = await loadPlugins([
      definePlugin({
        name: "a",
        version: "1.0.0",
        setup: () => {
          order.push("a");
        },
      }),
      definePlugin({
        name: "b",
        version: "1.0.0",
        setup: () => {
          throw new Error("nope");
        },
      }),
      definePlugin({
        name: "c",
        version: "1.0.0",
        setup: () => {
          order.push("c");
        },
      }),
    ]);

    expect(order).toEqual(["a", "c"]);
    expect(results.map((r) => r.kind)).toEqual(["loaded", "failed", "loaded"]);
    expect(usePluginStore.getState().loaded.has("a")).toBe(true);
    expect(usePluginStore.getState().loaded.has("b")).toBe(false);
    expect(usePluginStore.getState().loaded.has("c")).toBe(true);
  });

  it("honours `requires` and reorders to satisfy them", async () => {
    const order: string[] = [];
    await loadPlugins([
      // Declared first, but requires `b` — must load second.
      definePlugin({
        name: "a",
        version: "1.0.0",
        requires: ["b"],
        setup: () => {
          order.push("a");
        },
      }),
      definePlugin({
        name: "b",
        version: "1.0.0",
        setup: () => {
          order.push("b");
        },
      }),
    ]);
    expect(order).toEqual(["b", "a"]);
  });

  it("preserves manifest order between independent plugins", async () => {
    const order: string[] = [];
    await loadPlugins([
      definePlugin({
        name: "x",
        version: "1.0.0",
        setup: () => {
          order.push("x");
        },
      }),
      definePlugin({
        name: "y",
        version: "1.0.0",
        setup: () => {
          order.push("y");
        },
      }),
      definePlugin({
        name: "z",
        version: "1.0.0",
        setup: () => {
          order.push("z");
        },
      }),
    ]);
    expect(order).toEqual(["x", "y", "z"]);
  });

  it("skips a plugin whose required dep isn't loaded", async () => {
    const results = await loadPlugins([
      definePlugin({
        name: "orphan",
        version: "1.0.0",
        requires: ["does-not-exist"],
        setup: vi.fn(),
      }),
    ]);
    const first = results[0]!;
    expect(first).toMatchObject({ kind: "skipped", name: "orphan" });
    if (first.kind !== "skipped") throw new Error("unreachable");
    expect(first.reason).toMatch(/does-not-exist/);
  });

  it("skips every plugin participating in a dependency cycle", async () => {
    const setupA = vi.fn();
    const setupB = vi.fn();
    const results = await loadPlugins([
      definePlugin({ name: "a", version: "1.0.0", requires: ["b"], setup: setupA }),
      definePlugin({ name: "b", version: "1.0.0", requires: ["a"], setup: setupB }),
    ]);
    expect(setupA).not.toHaveBeenCalled();
    expect(setupB).not.toHaveBeenCalled();
    expect(results.every((r) => r.kind === "skipped")).toBe(true);
  });
});

describe("lazy activation", () => {
  it("stages the placeholder and skips setup until activation", async () => {
    const setup = vi.fn();
    await loadPlugins([
      definePlugin({
        name: "lazy.plugin",
        version: "1.0.0",
        activationEvents: ["onCommand:lazy.do"],
        contributes: { commands: [{ id: "lazy.do", label: "Lazy: Do It" }] },
        setup,
      }),
    ]);

    expect(setup).not.toHaveBeenCalled();
    expect(usePluginStore.getState().declaredCommands.has("lazy.do")).toBe(true);
    expect(usePluginStore.getState().pendingActivations.has("lazy.plugin")).toBe(true);
    // lookupCommand only sees *registered* commands, so still undefined.
    expect(lookupCommand("lazy.do")).toBeUndefined();
  });

  it("runs setup-returned cleanup when the plugin is unloaded", async () => {
    const cleanup = vi.fn();
    await loadPlugin(
      definePlugin({
        name: "with-cleanup",
        version: "1.0.0",
        setup: ({ host }) => {
          host.commands.register({ id: "with-cleanup.cmd", label: "x", run: () => {} });
          return cleanup;
        },
      }),
    );
    expect(usePluginStore.getState().loaded.has("with-cleanup")).toBe(true);
    expect(lookupCommand("with-cleanup.cmd")).toBeDefined();

    unloadPlugin("with-cleanup");

    expect(cleanup).toHaveBeenCalledOnce();
    expect(usePluginStore.getState().loaded.has("with-cleanup")).toBe(false);
    // host.commands.register's disposable also ran — its entry is gone.
    expect(lookupCommand("with-cleanup.cmd")).toBeUndefined();
  });

  it("restricts host to declared capabilities and throws on disallowed namespaces", async () => {
    let captured: unknown = null;
    const result = await loadPlugin(
      definePlugin({
        name: "capped",
        version: "1.0.0",
        capabilities: ["commands"],
        setup: ({ host }) => {
          host.commands.register({ id: "ok", label: "OK", run: () => {} });
          try {
            // host.tool is not declared — must throw.
            (
              host as unknown as Record<string, { registerPreview: () => void }>
            ).tool!.registerPreview();
          } catch (err) {
            captured = err;
          }
        },
      }),
    );
    expect(result.kind).toBe("loaded");
    expect(lookupCommand("ok")).toBeDefined();
    expect(captured).toBeInstanceOf(Error);
    expect(String(captured)).toMatch(/not in this plugin's declared capabilities/);
  });

  it("reload disposes + re-runs setup", async () => {
    const setup = vi.fn(({ host }) => {
      host.commands.register({ id: "reload.test", label: "x", run: () => {} });
    });
    await loadPlugin(definePlugin({ name: "reloadable", version: "1.0.0", setup }));
    expect(setup).toHaveBeenCalledOnce();
    await reloadPlugin("reloadable");
    expect(setup).toHaveBeenCalledTimes(2);
    expect(lookupCommand("reload.test")).toBeDefined();
  });

  it("runs setup + dispatches the real handler when the placeholder fires", async () => {
    const handler = vi.fn();
    await loadPlugins([
      definePlugin({
        name: "lazy.greet",
        version: "1.0.0",
        activationEvents: ["onCommand:greet"],
        contributes: { commands: [{ id: "greet", label: "Greet" }] },
        setup: ({ host }) => {
          host.commands.register({ id: "greet", label: "Greet", run: handler });
        },
      }),
    ]);

    // Activate by name, just like the placeholder's `run` does.
    const store = usePluginStore.getState();
    const pending = store.pendingActivations.get("lazy.greet");
    if (!pending) throw new Error("pending entry missing");
    store.removePendingActivation("lazy.greet");
    await loadPlugin(pending.spec);
    store.removeDeclaredCommandsBy("lazy.greet");

    // After activation, the real command is registered and dispatchable.
    const real = lookupCommand("greet");
    expect(real).toBeDefined();
    await real!.run();
    expect(handler).toHaveBeenCalledOnce();

    // And the placeholder is gone.
    expect(usePluginStore.getState().declaredCommands.has("greet")).toBe(false);
  });
});
