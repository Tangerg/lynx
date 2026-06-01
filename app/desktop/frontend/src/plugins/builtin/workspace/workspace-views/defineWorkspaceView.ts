// Convenience wrapper for the built-in workspace-view plugins: each is just
// `definePlugin` → `contribute(WORKSPACE_VIEW, spec)` with a name derived from
// the view id. Lives in this plugin package — NOT the core SDK — mirroring
// `defineThemePlugin` in theme/kit: the kernel exposes only the generic
// `contribute` write path; per-domain ergonomics belong to the domain.

import type { PluginSpec, WorkspaceViewSpec } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";

export function defineWorkspaceView(spec: WorkspaceViewSpec): PluginSpec {
  return definePlugin({
    name: `lyra.builtin.view-${spec.id}`,
    version: "1.0.0",
    setup({ host }) {
      host.extensions.contribute(WORKSPACE_VIEW, spec);
    },
  });
}
