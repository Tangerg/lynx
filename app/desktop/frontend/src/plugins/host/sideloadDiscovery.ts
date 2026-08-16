// Sideload discovery: ask the Wails host what is installed, hand each one to the
// platform. Registration failures are per-plugin and reported there.

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

export async function loadSideloadedPlugins(): Promise<number> {
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
  if (!sources.length) return 0;

  createSideloadPlatform(GRANTABLE);

  let loaded = 0;
  for (const info of sources) {
    // The blob URL stays alive for the session: the platform re-imports it when
    // a lazily-activated plugin is finally triggered, and revoking here would
    // leave that activation with nothing to load.
    const url = URL.createObjectURL(new Blob([info.source], { type: "text/javascript" }));
    let manifest: unknown;
    try {
      manifest = ((await import(/* @vite-ignore */ url)) as { manifest?: unknown }).manifest;
    } catch (err) {
      console.error(`[plugin] sideload ${info.id} import failed:`, err);
      continue;
    }
    const parsed = sideloadManifestSchema.safeParse(manifest);
    if (!parsed.success) {
      console.warn(
        `[plugin] sideload ${info.id} has no valid \`manifest\` export:`,
        z.treeifyError(parsed.error),
      );
      continue;
    }
    if (await registerSideloadedPlugin(parsed.data, url)) loaded += 1;
  }
  return loaded;
}
