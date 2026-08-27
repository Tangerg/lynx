import type { AnyPlugin } from "dougong";
import { defineVisualStylePlugin } from "./defineVisualStylePlugin";
import { scopeappStyle } from "./scopeapp";

export const builtinVisualStyleSpecs = [scopeappStyle] as const;

export const builtinVisualStyles: AnyPlugin[] =
  builtinVisualStyleSpecs.map(defineVisualStylePlugin);
