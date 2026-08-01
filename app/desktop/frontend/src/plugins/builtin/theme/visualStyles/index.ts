import type { PluginSpec } from "@/plugins/sdk";
import { defineVisualStylePlugin } from "./defineVisualStylePlugin";
import { herouiStyle } from "./heroui";
import { jetbrainsClassicStyle } from "./jetbrainsClassic";
import { jetbrainsHeroStyle } from "./jetbrainsHero";
import { novaStyle } from "./nova";
import { synaraStyle } from "./synara";
import { zedStyle } from "./zed";

export const builtinVisualStyleSpecs = [
  synaraStyle,
  novaStyle,
  zedStyle,
  jetbrainsClassicStyle,
  jetbrainsHeroStyle,
  herouiStyle,
] as const;

export const builtinVisualStyles: PluginSpec[] =
  builtinVisualStyleSpecs.map(defineVisualStylePlugin);
