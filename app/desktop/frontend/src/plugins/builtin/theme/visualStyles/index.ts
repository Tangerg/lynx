import type { AnyPlugin } from "dougong";
import { defineVisualStylePlugin } from "./defineVisualStylePlugin";
import { lyraStyle } from "./lyra";

export const builtinVisualStyleSpecs = [lyraStyle] as const;

export const builtinVisualStyles: AnyPlugin[] =
  builtinVisualStyleSpecs.map(defineVisualStylePlugin);
