import type { Host, Platform, Registration } from "dougong";
import { afterEach, describe, expect, it, vi } from "vitest";
import { startKernel, stopKernel } from "./bootstrap";
import { definePlugin } from "./definePlugin";
import { installedPluginRecords, installedPlugins, removeInstallation } from "./kernel";
import { registerSideloadedPlugin } from "./sideload";

let host: Host | undefined;

afterEach(async () => {
  if (!host) return;
  const owned = host;
  host = undefined;
  await stopKernel(owned);
});

describe("sideload installation read model", () => {
  it("publishes a successful Platform registration and removes the same handle", async () => {
    host = await startKernel([]);
    const remove = vi.fn().mockResolvedValue(undefined);
    const registration = { remove } as unknown as Registration<string | URL>;
    const platform = {
      register: vi.fn().mockResolvedValue(registration),
    } as unknown as Platform<string | URL>;

    await expect(
      registerSideloadedPlugin(
        platform,
        host,
        { name: "third.party", version: "1.0.0" },
        "blob:third-party",
      ),
    ).resolves.toBe(true);

    expect(installedPlugins()).toEqual(["third.party"]);
    expect(installedPluginRecords()).toEqual([{ name: "third.party", origin: "sideload" }]);
    await removeInstallation("third.party");
    expect(remove).toHaveBeenCalledOnce();
    expect(installedPlugins()).toEqual([]);
  });

  it("rejects a sideload name collision without changing the built-in origin", async () => {
    host = await startKernel([definePlugin({ name: "lyra.builtin.existing", setup() {} })]);
    const register = vi.fn();
    const platform = { register } as unknown as Platform<string | URL>;

    await expect(
      registerSideloadedPlugin(
        platform,
        host,
        { name: "lyra.builtin.existing", version: "1.0.0" },
        "blob:spoofed",
      ),
    ).resolves.toBe(false);

    expect(register).not.toHaveBeenCalled();
    expect(installedPluginRecords()).toEqual([
      { name: "lyra.builtin.existing", origin: "builtin" },
    ]);
  });
});
