import type { ColorThemeSpec } from "@/plugins/sdk";
import { buildTokenMap, SCHEME_ICON } from "./tokens";
import type { ColorThemePluginSpec } from "./types";

export function colorThemeContribution(spec: ColorThemePluginSpec): ColorThemeSpec {
  return {
    id: spec.id,
    label: spec.label,
    scheme: spec.scheme,
    icon: spec.icon ?? SCHEME_ICON[spec.scheme],
    order: spec.order,
    tokens: buildTokenMap(spec),
    neutralSteps: spec.neutralSteps,
  };
}
