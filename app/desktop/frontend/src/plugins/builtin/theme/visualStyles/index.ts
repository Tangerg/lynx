import type { PluginSpec } from "@/plugins/sdk";
import { defineVisualStylePlugin } from "./defineVisualStylePlugin";
import { lyraStyle } from "./lyra";

export const builtinVisualStyleSpecs = [lyraStyle] as const;

export const builtinVisualStyles: PluginSpec[] =
  builtinVisualStyleSpecs.map(defineVisualStylePlugin);
