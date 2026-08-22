// The kernel's contribution read is the whole read side's foundation, so these
// drive a real Host rather than a stub: the questions worth asking (when does a
// contribution become visible, what happens to a shadowed one when its shadow
// unloads) are answered by Core's transaction, not by our code.

import { createHost, type AnyPlugin, type Host } from "dougong";
import { afterEach, describe, expect, it } from "vitest";
import { defineExtensionPoint } from "./contracts";
import { contributionsTo, publishKernel, retractKernel, subscribeContributions } from "./kernel";
import { definePlugin } from "./definePlugin";

interface Theme {
  id: string;
  label: string;
  order?: number;
}

const THEME = defineExtensionPoint<Theme>({ id: "test.theme", keying: "single" });

let host: Host | undefined;

afterEach(async () => {
  if (host) {
    retractKernel(host);
    await host.stop();
  }
  host = undefined;
});

// A point is read straight off the Host, so nothing has to be declared up front;
// the array survives only as what the assertions read back through.
function stand(plugins: AnyPlugin[]): Host {
  const next = createHost({ name: "test", onError: () => {} });
  for (const plugin of plugins) next.install(plugin);
  return next;
}

const index = { entries: contributionsTo };

async function start(plugins: AnyPlugin[]) {
  host = stand(plugins);
  await host.start();
  publishKernel(host);
  return index;
}

describe("kernel contribution reads", () => {
  it("publishes a contribution under its domain key, not Core's owner-qualified one", async () => {
    const contributor = definePlugin({
      name: "test.contributor",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Dark" });
      },
    });

    const index = await start([contributor]);

    expect(index.entries(THEME).map((e) => e.key)).toEqual(["dark"]);
    expect(index.entries(THEME)[0]?.plugin).toBe("test.contributor");
  });

  it("sorts by the item's own order ahead of the contribute-time hint", async () => {
    const plugin = definePlugin({
      name: "test.many",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "c", label: "C" }, { order: 30 });
        ctx.contribute(THEME, { id: "a", label: "A", order: 1 }, { order: 99 });
        ctx.contribute(THEME, { id: "b", label: "B" }, { order: 20 });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(THEME).map((e) => e.item.id)).toEqual(["a", "b", "c"]);
  });

  it("gives a single point's key to the last contributor", async () => {
    const base = definePlugin({
      name: "test.base",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Base" });
      },
    });
    const override = definePlugin({
      name: "test.override",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Override" });
      },
    });

    const index = await start([base, override]);

    expect(index.entries(THEME).map((e) => e.item.label)).toEqual(["Override"]);
  });

  it("keeps every contribution to a multi point, including one plugin's duplicates", async () => {
    const HANDLER = defineExtensionPoint<{ run: () => void }>({
      id: "test.handler",
      keying: "multi",
    });
    const plugin = definePlugin({
      name: "test.handlers",
      setup: (ctx) => {
        ctx.contribute(HANDLER, { run: () => {} });
        ctx.contribute(HANDLER, { run: () => {} });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(HANDLER)).toHaveLength(2);
  });

  it("returns the same array reference until something changes", async () => {
    const plugin = definePlugin({
      name: "test.stable",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Dark" });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(THEME)).toBe(index.entries(THEME));
  });

  it("reads empty for a point nothing contributed to, without minting a new array", async () => {
    const UNDECLARED = defineExtensionPoint<Theme>({ id: "test.undeclared", keying: "single" });

    const index = await start([]);

    expect(index.entries(UNDECLARED)).toEqual([]);
    expect(index.entries(UNDECLARED)).toBe(index.entries(UNDECLARED));
  });

  it("notifies subscribers when a later change adds a contribution", async () => {
    const base = definePlugin({
      name: "test.base",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Base" });
      },
    });
    const late = definePlugin({
      name: "test.late",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "light", label: "Light" });
      },
    });

    const index = await start([base]);
    let notified = 0;
    const stop = subscribeContributions(() => notified++);
    // Reading is what binds the point's view, and the subscription with it.
    index.entries(THEME);

    const change = host!.change();
    change.install(late);
    await change.commit();

    expect(notified).toBeGreaterThan(0);
    expect(index.entries(THEME).map((e) => e.key)).toEqual(["dark", "light"]);
    stop();
  });

  it("restores a shadowed contribution when the plugin shadowing it is removed", async () => {
    const base = definePlugin({
      name: "test.base",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Base" });
      },
    });
    const override = definePlugin({
      name: "test.override",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Override" });
      },
    });

    host = stand([base]);
    const installed = host.install(override);
    await host.start();
    publishKernel(host);
    expect(index.entries(THEME).map((e) => e.item.label)).toEqual(["Override"]);

    await installed.remove();

    expect(index.entries(THEME).map((e) => e.item.label)).toEqual(["Base"]);
  });
});

describe("contribute policy", () => {
  it("derives a single point's key from keyOf ahead of item.id", async () => {
    const ICON = defineExtensionPoint<{ id: string; fn: string }>({
      id: "test.icon",
      keying: "single",
      keyOf: (item) => item.fn,
    });
    const plugin = definePlugin({
      name: "test.icons",
      setup: (ctx) => {
        ctx.contribute(ICON, { id: "ignored", fn: "read_file" });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(ICON)[0]?.key).toBe("read_file");
  });

  it("normalizes a key so a registration and a lookup of the same combo agree", async () => {
    const SLASH = defineExtensionPoint<{ label: string }>({
      id: "test.slash",
      keying: "single",
      normalizeKey: (key) => (key.startsWith("/") ? key : `/${key}`),
    });
    const plugin = definePlugin({
      name: "test.slashes",
      setup: (ctx) => {
        ctx.contribute(SLASH, { label: "Ping" }, { key: "ping" });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(SLASH)[0]?.key).toBe("/ping");
  });

  it("refuses a single contribution with no key to be found anywhere", async () => {
    const KEYLESS = defineExtensionPoint<{ label: string }>({
      id: "test.keyless",
      keying: "single",
    });
    const plugin = definePlugin({
      name: "test.keyless",
      setup: (ctx) => {
        ctx.contribute(KEYLESS, { label: "no id" });
      },
    });

    host = stand([plugin]);

    await expect(host.start()).rejects.toThrow(/requires opts.key, keyOf, or a non-empty item.id/);
  });
});
