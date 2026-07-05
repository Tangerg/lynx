import type { ThemeSpec } from "@/plugins/sdk";
import { buildTokenMap, SCHEME_ICON } from "./tokens";
import type { ThemePluginSpec } from "./types";

export function themeContribution(spec: ThemePluginSpec): ThemeSpec {
  return {
    id: spec.id,
    label: spec.label,
    scheme: spec.scheme,
    icon: spec.icon ?? SCHEME_ICON[spec.scheme],
    order: spec.order,
    tokens: buildTokenMap(spec),
  };
}
