// Tool preview router — looks up the renderer in the plugin registry.
//
// Pre-plugin version was a hard-coded `if (fn === "bash") ... else if ...`
// switch. Now every fn → component mapping lives in a plugin, including the
// built-ins (see plugins/builtin/*). No special-casing for "this is built-in".

import { PluginBoundary } from "@/plugins/PluginBoundary";
import { useToolPreview } from "@/plugins/sdk";
import type { ToolCall } from "@/protocol/agui/viewState";

type Props = {
  tool: ToolCall;
  onOpenInspector: () => void;
};

export function ToolPreview({ tool, onOpenInspector }: Props) {
  const Preview = useToolPreview(tool.fn);
  if (!Preview) return null;
  return (
    <PluginBoundary plugin={tool.fn} label={`${tool.fn} preview`}>
      <Preview tool={tool} onOpenInspector={onOpenInspector} />
    </PluginBoundary>
  );
}
