// Test harness: a real Host, because the questions tests ask about plugins
// (does this contribution show up, does that handler run) are answered by Core's
// transaction and not by anything we could stub convincingly.

import type { AnyPlugin, Host } from "dougong";
import { definePlugin, type PluginContext } from "./definePlugin";
import { startKernel, stopKernel } from "./bootstrap";
import { trackInstalledPlugin } from "./kernel";

let running: Host | undefined;

/**
 * Add plugins to the spec's kernel, booting one if this is the first call.
 *
 * Additive, because a fixture builds its world in several calls and a
 * replace-all would run the previous batch's cleanups — unbinding the ports the
 * next batch renders through. `src/test/setup.ts` tears the kernel down between
 * specs, so nothing leaks across them.
 */
export async function loadPluginsForTest(...plugins: AnyPlugin[]): Promise<Host> {
  if (running) {
    await addPluginsForTest(running, plugins);
    return running;
  }
  running = await startKernel(plugins);
  return running;
}

export async function addPluginsForTest(
  host: Host,
  plugins: ReadonlyArray<AnyPlugin>,
): Promise<void> {
  if (!plugins.length) return;
  const change = host.change();
  for (const plugin of plugins) change.install(plugin);
  await change.commit();
  for (const plugin of plugins) trackInstalledPlugin(host, plugin.name);
}

/** Boot a kernel whose only plugin contributes whatever the test needs. */
export async function contributeForTest(
  setup: (ctx: PluginContext) => void,
  name = "test.contributor",
): Promise<Host> {
  return loadPluginsForTest(definePlugin({ name, setup }));
}

export async function resetKernelForTest(): Promise<void> {
  if (!running) return;
  const host = running;
  running = undefined;
  await stopKernel(host);
}
