import type { VisualStyleSpec } from "@/plugins/sdk";
import type { AnyPlugin } from "dougong";
import { definePlugin } from "@/plugins/sdk";
import { VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";

export function defineVisualStylePlugin(spec: VisualStyleSpec): AnyPlugin {
  return definePlugin({
    name: `scopeapp.builtin.visual-style-${spec.id}`,
    setup(ctx) {
      ctx.contribute(VISUAL_STYLE, spec);
    },
  });
}
