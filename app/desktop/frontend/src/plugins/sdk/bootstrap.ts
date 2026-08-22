// Standing a kernel up. Built-ins go in as one transaction — a built-in that
// throws is a broken build, and a Host that refuses to start says so, where the
// old loader booted two thirds of an app and left the user to work out which
// third was missing.

import { createHost, type AnyPlugin, type Host } from "dougong";
import { reportPluginError } from "./errors";
import { kernelLogger } from "./hostLog";
import { contributionsTo, publishKernel, retractKernel, trackInstalledPlugin } from "./kernel";
import { READY_HANDLER } from "./kernelPoints";
import { shellServices } from "./shellServices";

export function createKernel(plugins: ReadonlyArray<AnyPlugin>): Host {
  const host = createHost({
    name: "lyra",
    logger: kernelLogger,
    onError: (error) => reportPluginError("kernel", "setup", error),
  });
  host.install(shellServices);
  for (const plugin of plugins) {
    host.install(plugin);
    trackInstalledPlugin(host, plugin.name);
  }
  return host;
}

export async function startKernel(
  plugins: ReadonlyArray<AnyPlugin>,
  signal?: AbortSignal,
): Promise<Host> {
  const host = createKernel(plugins);
  try {
    signal?.throwIfAborted();
    await host.start();
    signal?.throwIfAborted();
    publishKernel(host);
    fireReadyHandlers();
    return host;
  } catch (error) {
    try {
      await host.stop();
    } catch (stopError) {
      throw new AggregateError([error, stopError], "Kernel startup and rollback both failed");
    }
    throw error;
  }
}

export async function stopKernel(host: Host): Promise<void> {
  retractKernel(host);
  await host.stop();
}

// `host.start()` resolving is the ready point; Core has no post-start hook
// because it has no opinion about what "the app is up" means.
function fireReadyHandlers(): void {
  for (const entry of contributionsTo(READY_HANDLER)) {
    try {
      entry.item();
    } catch (error) {
      reportPluginError(entry.plugin, "setup", error);
    }
  }
}
