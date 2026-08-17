// Third-party plugins, on the platform layer.
//
// Each registration is its own transaction, so a broken third-party plugin rolls
// back alone and the running app never notices — the opposite of the built-in
// set, which is deliberately all-or-nothing.
//
// Laziness is the platform's: a manifest declares its activation events, an
// artifact carries a `placeholder` plugin that contributes the surfaces the real
// module will contribute (behind an activating stub), and `trigger` swaps the
// placeholder for the module. The kernel keeps no separate table of "declared
// but not yet real" contributions; the placeholder's contributions ARE real, and
// replacing them is an ordinary change.

import {
  createPlatform,
  ImportLoader,
  PermissionSet,
  type Platform,
  type AnyPlugin,
  type Host,
} from "dougong";
import { z } from "zod";
import { HOST_API_VERSION } from "./apiVersion";
import { definePlugin } from "./definePlugin";
import { measurePluginLoad } from "@/lib/metrics";
import { reportPluginError } from "./errors";
import { hasInstallation, trackInstallation } from "./kernel";
import { COMMAND, SETTINGS_PANE, WORKSPACE_VIEW } from "./kernelPoints";
import { makeLazyActivator } from "./lazyActivator";
import type { ContributedCommand } from "./types/commands";
import type { ContributedSettingsPane, ContributedView } from "./types/declared";

const declaredSchema = z.object({
  commands: z.array(z.custom<ContributedCommand>()).optional(),
  views: z.array(z.custom<ContributedView>()).optional(),
  settingsPanes: z.array(z.custom<ContributedSettingsPane>()).optional(),
});

export const sideloadManifestSchema = z.object({
  name: z.string().min(1),
  version: z.string().min(1),
  apiVersion: z.string().optional(),
  activation: z.array(z.string()).optional(),
  permissions: z.array(z.string()).optional(),
  dependencies: z.record(z.string(), z.string()).optional(),
  contributes: declaredSchema.optional(),
});

export type SideloadManifest = z.infer<typeof sideloadManifestSchema>;

/** Capabilities a third-party plugin may be granted. Anything outside this set
 *  is denied at registration, before a line of its code runs. */
export function createSideloadPlatform(
  allowed: Iterable<string>,
  installer: Host,
): Platform<string | URL> {
  return createPlatform<string | URL>({
    installer,
    apiVersion: HOST_API_VERSION,
    loader: new ImportLoader(),
    authorizer: new PermissionSet(allowed),
  });
}

/** Fire an activation event; every artifact waiting on it activates. */
async function triggerActivation(platform: Platform<string | URL>, event: string): Promise<void> {
  await platform.trigger(event);
}

/**
 * The plugin that stands in until the real module loads. It contributes the
 * surfaces the manifest declared, each rendering an activating stub, so the
 * entry is visible and clickable before any third-party code has run.
 */
function placeholderFor(
  platform: Platform<string | URL>,
  manifest: SideloadManifest,
  event: string,
): AnyPlugin {
  const declared = manifest.contributes ?? {};
  return definePlugin({
    name: manifest.name,
    setup(ctx) {
      for (const command of declared.commands ?? []) {
        ctx.contribute(COMMAND, {
          ...command,
          run: () => void triggerActivation(platform, event),
        });
      }
      for (const view of declared.views ?? []) {
        ctx.contribute(WORKSPACE_VIEW, {
          ...view,
          component: makeLazyActivator(view.title, () => void triggerActivation(platform, event)),
        });
      }
      for (const pane of declared.settingsPanes ?? []) {
        ctx.contribute(SETTINGS_PANE, {
          ...pane,
          component: makeLazyActivator(pane.label, () => void triggerActivation(platform, event)),
        });
      }
    },
  });
}

export async function registerSideloadedPlugin(
  platform: Platform<string | URL>,
  owner: Host,
  manifest: SideloadManifest,
  reference: string | URL,
  signal?: AbortSignal,
): Promise<boolean> {
  const activation = manifest.activation?.length ? manifest.activation : ["startup"];
  const started = performance.now();
  try {
    if (hasInstallation(owner, manifest.name)) {
      throw new Error(`Plugin "${manifest.name}" is already installed in this Host generation`);
    }
    const registration = await platform.register({
      manifest: {
        name: manifest.name,
        version: manifest.version,
        apiVersion: manifest.apiVersion ?? HOST_API_VERSION,
        activation,
        permissions: manifest.permissions ?? [],
        dependencies: manifest.dependencies ?? {},
      },
      reference,
      ...(activation.includes("startup")
        ? {}
        : { placeholder: placeholderFor(platform, manifest, activation[0]!) }),
    });
    if (signal?.aborted) return false;
    trackInstallation(owner, manifest.name, registration, "sideload");
    measurePluginLoad(performance.now() - started, manifest.name, "loaded");
    return true;
  } catch (error) {
    if (signal?.aborted) return false;
    measurePluginLoad(performance.now() - started, manifest.name, "failed");
    reportPluginError(manifest.name, "setup", error);
    return false;
  }
}
