import type { Host } from "dougong";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createKernel, startKernel, stopKernel } from "./bootstrap";
import { definePlugin } from "./definePlugin";
import {
  installedPlugins,
  kernelHost,
  removeInstallation,
  retractKernel,
  trackInstallation,
} from "./kernel";

const hosts: Host[] = [];

afterEach(async () => {
  await Promise.allSettled(
    hosts.splice(0).map(async (host) => {
      retractKernel(host);
      await host.stop();
    }),
  );
});

function plugin(name: string) {
  return definePlugin({ name, setup() {} });
}

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

describe("kernel generation ownership", () => {
  it("publishes only the successor generation's installation read model", async () => {
    hosts.push(await startKernel([plugin("test.old")]));
    hosts.push(await startKernel([plugin("test.successor")]));

    expect(installedPlugins()).toEqual(["test.successor"]);
  });

  it("does not let a stale stop retract the successor generation", async () => {
    const old = await startKernel([plugin("test.old")]);
    const successor = await startKernel([plugin("test.successor")]);
    hosts.push(old, successor);

    await stopKernel(old);

    expect(kernelHost()).toBe(successor);
    expect(installedPlugins()).toEqual(["test.successor"]);
  });

  it("keeps unpublished installation handles isolated from the active generation", async () => {
    const active = await startKernel([plugin("test.active")]);
    const unpublished = createKernel([plugin("test.unpublished")]);
    hosts.push(active, unpublished);

    expect(kernelHost()).toBe(active);
    expect(installedPlugins()).toEqual(["test.active"]);
  });

  it("rolls back a startup that was retired before its non-cooperative setup settled", async () => {
    const setup = deferred();
    const cleanup = vi.fn();
    const controller = new AbortController();
    const starting = startKernel(
      [
        definePlugin<{}, {}>({
          name: "test.delayed",
          async setup(ctx) {
            ctx.cleanup(cleanup);
            await setup.promise;
          },
        }),
      ],
      controller.signal,
    );

    controller.abort();
    setup.resolve();

    await expect(starting).rejects.toThrow(/aborted/i);
    expect(cleanup).toHaveBeenCalledOnce();
    expect(kernelHost).toThrow(/No kernel is running/);
    expect(installedPlugins()).toEqual([]);
  });

  it("keeps a failed removal visible until its transaction actually commits", async () => {
    const active = await startKernel([]);
    hosts.push(active);
    trackInstallation(active, "test.failing-removal", {
      remove: vi.fn().mockRejectedValue(new Error("commit failed")),
    });

    await expect(removeInstallation("test.failing-removal")).rejects.toThrow("commit failed");

    expect(installedPlugins()).toEqual(["test.failing-removal"]);
  });

  it("does not let an old removal settlement delete a replacement handle", async () => {
    const active = await startKernel([]);
    const removal = deferred();
    hosts.push(active);
    trackInstallation(active, "test.replaceable", { remove: () => removal.promise });
    const removing = removeInstallation("test.replaceable");

    trackInstallation(active, "test.replaceable", { remove: vi.fn() });
    removal.resolve();
    await removing;

    expect(installedPlugins()).toEqual(["test.replaceable"]);
  });
});
