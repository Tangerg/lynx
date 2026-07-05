import type { IconName } from "@/ui/icons";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { lookupExtensionByKey, TOOL_ICON } from "@/plugins/sdk";
import { defaultToolIconFor } from "../application/toolIconContributions";

export { defaultToolIconContributions } from "../application/toolIconContributions";

/** The icon/preview registry key for a tool = its wire `name` (§4.4). */
export function toolRoutingKey(tool: ToolCall): string {
  return tool.name;
}

export function toolIconFor(key: string): IconName {
  const registered = lookupExtensionByKey(TOOL_ICON, key);
  if (registered) return registered as IconName;
  return defaultToolIconFor(key) as IconName;
}
