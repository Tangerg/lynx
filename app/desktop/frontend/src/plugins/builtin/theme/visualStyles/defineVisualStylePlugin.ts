import type { PluginSpec, VisualStyleSpec } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";

export function defineVisualStylePlugin(spec: VisualStyleSpec): PluginSpec {
  return definePlugin({
    name: `lyra.builtin.visual-style-${spec.id}`,
    version: "1.0.0",
    setup({ host }) {
      host.extensions.contribute(VISUAL_STYLE, spec);
    },
  });
}
