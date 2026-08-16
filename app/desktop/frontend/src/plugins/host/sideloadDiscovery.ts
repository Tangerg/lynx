// Sideload discovery: ask the Wails host what is installed, hand each one to the
// platform. Registration failures are per-plugin and reported there.

import type { Host, Platform } from "dougong";
import { z } from "zod";
import { getContainer } from "@/main/container";
import { CAPABILITY_RISK } from "../sdk/capabilities";
import {
  createSideloadPlatform,
  registerSideloadedPlugin,
  sideloadManifestSchema,
} from "../sdk/sideload";

// The capability vocabulary is the risk table's keys; anything a manifest asks
// for outside it is denied before a line of the plugin's code runs.
const GRANTABLE = Object.keys(CAPABILITY_RISK);

export interface SideloadDiscovery {
  readonly completion: Promise<number>;
  dispose(): Promise<void>;
}

export function loadSideloadedPlugins(host: Host): SideloadDiscovery {
  const controller = new AbortController();
  const urls = new Set<string>();
  let platform: Platform<string | URL> | undefined;
  let disposal: Promise<void> | undefined;

  const releaseUrl = (url: string) => {
    if (!urls.delete(url)) return;
    URL.revokeObjectURL(url);
  };
  const releaseUrls = () => {
    for (const url of [...urls]) releaseUrl(url);
  };
  const dispose = () => {
    if (disposal) return disposal;
    controller.abort();
    const ownedPlatform = platform;
    disposal = (async () => {
      try {
        await ownedPlatform?.dispose();
      } finally {
        releaseUrls();
      }
    })();
    return disposal;
  };

  const completion = discover().catch((error: unknown) => {
    if (!controller.signal.aborted) {
      console.warn("[plugin] sideload discovery failed:", error);
    }
    return 0;
  });

  return { completion, dispose };

  async function discover(): Promise<number> {
    let sources: Array<{ id: string; source: string }>;
    try {
      const bootstrap = await getContainer().desktop.bootstrap();
      sources = bootstrap?.sideloadedPlugins ?? [];
      for (const issue of bootstrap?.sideloadIssues ?? []) {
        console.warn(`[plugin] sideload ${issue.id} was skipped: ${issue.detail}`);
      }
    } catch (err) {
      console.warn("[plugin] desktop host bootstrap failed:", err);
      return 0;
    }
    if (controller.signal.aborted) return 0;
    if (!sources.length) return 0;

    platform = createSideloadPlatform(GRANTABLE, host);

    let loaded = 0;
    for (const info of sources) {
      if (controller.signal.aborted) break;
      // Successful registrations retain the blob until this generation is
      // disposed because lazy activation may import it later. Failed candidates
      // release theirs immediately.
      const url = URL.createObjectURL(new Blob([info.source], { type: "text/javascript" }));
      urls.add(url);
      let manifest: unknown;
      try {
        manifest = ((await import(/* @vite-ignore */ url)) as { manifest?: unknown }).manifest;
      } catch (err) {
        if (!controller.signal.aborted) {
          console.error(`[plugin] sideload ${info.id} import failed:`, err);
        }
        releaseUrl(url);
        continue;
      }
      if (controller.signal.aborted) {
        releaseUrl(url);
        break;
      }
      const parsed = sideloadManifestSchema.safeParse(manifest);
      if (!parsed.success) {
        console.warn(
          `[plugin] sideload ${info.id} has no valid \`manifest\` export:`,
          z.treeifyError(parsed.error),
        );
        releaseUrl(url);
        continue;
      }
      if (await registerSideloadedPlugin(platform, parsed.data, url, controller.signal)) {
        loaded += 1;
      } else {
        releaseUrl(url);
      }
    }
    return loaded;
  }
}
