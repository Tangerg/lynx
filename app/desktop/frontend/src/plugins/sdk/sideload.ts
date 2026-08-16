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
} from "dougong";
import { z } from "zod";
import { HOST_API_VERSION } from "./apiVersion";
import { definePlugin } from "./definePlugin";
import { measurePluginLoad } from "@/lib/metrics";
import { reportPluginError } from "./errors";
import { kernelHost } from "./kernel";
import { COMMAND, SETTINGS_PANE, WORKSPACE_VIEW } from "./kernelPoints";
import { makeLazyActivator } from "./lazyActivator";
import { setPluginOrigin } from "./pluginOrigin";
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

let platform: Platform<string | URL> | undefined;

/** Capabilities a third-party plugin may be granted. Anything outside this set
 *  is denied at registration, before a line of its code runs. */
export function createSideloadPlatform(allowed: Iterable<string>): Platform<string | URL> {
  platform = createPlatform<string | URL>({
    installer: kernelHost(),
    apiVersion: HOST_API_VERSION,
    loader: new ImportLoader(),
    authorizer: new PermissionSet(allowed),
  });
  return platform;
}

export function sideloadPlatform(): Platform<string | URL> {
  if (!platform) throw new Error("No sideload platform — call createSideloadPlatform first");
  return platform;
}

/** Fire an activation event; every artifact waiting on it activates. */
export async function triggerActivation(event: string): Promise<void> {
  await platform?.trigger(event);
}

/**
 * The plugin that stands in until the real module loads. It contributes the
 * surfaces the manifest declared, each rendering an activating stub, so the
 * entry is visible and clickable before any third-party code has run.
 */
function placeholderFor(manifest: SideloadManifest, event: string): AnyPlugin {
  const declared = manifest.contributes ?? {};
  return definePlugin({
    name: manifest.name,
    setup(ctx) {
      for (const command of declared.commands ?? []) {
        ctx.contribute(COMMAND, {
          ...command,
          run: () => void triggerActivation(event),
        });
      }
      for (const view of declared.views ?? []) {
        ctx.contribute(WORKSPACE_VIEW, {
          ...view,
          component: makeLazyActivator(view.title, () => void triggerActivation(event)),
        });
      }
      for (const pane of declared.settingsPanes ?? []) {
        ctx.contribute(SETTINGS_PANE, {
          ...pane,
          component: makeLazyActivator(pane.label, () => void triggerActivation(event)),
        });
      }
    },
  });
}

export async function registerSideloadedPlugin(
  manifest: SideloadManifest,
  reference: string | URL,
): Promise<boolean> {
  const activation = manifest.activation?.length ? manifest.activation : ["startup"];
  setPluginOrigin(manifest.name, "sideload");
  const started = performance.now();
  try {
    await sideloadPlatform().register({
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
        : // `install` takes AnyPlugin in 0.2.0; `Artifact.placeholder` still wants
          // the fully-generic Plugin, so this one seam needs the cast.
          { placeholder: placeholderFor(manifest, activation[0]!) as never }),
    });
    measurePluginLoad(performance.now() - started, manifest.name, "loaded");
    return true;
  } catch (error) {
    measurePluginLoad(performance.now() - started, manifest.name, "failed");
    reportPluginError(manifest.name, "setup", error);
    return false;
  }
}
