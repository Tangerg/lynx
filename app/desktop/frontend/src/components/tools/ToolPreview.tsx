// Tool preview router — looks up the renderer in the plugin registry.
// Every fn → component mapping lives in a plugin (built-ins included);
// there is no special-casing for "this is built-in".

import { PluginBoundary } from "@/plugins/PluginBoundary";
import { useToolPreview } from "@/plugins/sdk";
import type { ToolCall } from "@/protocol/agui/viewState";

type Props = {
  tool: ToolCall;
  onOpenView: () => void;
};

export function ToolPreview({ tool, onOpenView }: Props) {
  const Preview = useToolPreview(tool.fn);
  if (!Preview) return null;
  return (
    <PluginBoundary plugin={tool.fn} label={`${tool.fn} preview`}>
      <Preview tool={tool} onOpenView={onOpenView} />
    </PluginBoundary>
  );
}
