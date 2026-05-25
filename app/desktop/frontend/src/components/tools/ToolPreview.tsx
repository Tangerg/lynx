// Tool preview router — looks up the renderer in the plugin registry.
// Every fn → component mapping lives in a plugin (built-ins included);
// there is no special-casing for "this is built-in".

import type { ToolCall } from "@/protocol/agui/viewState";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { useToolPreview } from "@/plugins/sdk";

interface Props {
  tool: ToolCall;
  onOpenView: () => void;
}

export function ToolPreview({ tool, onOpenView }: Props) {
  const Preview = useToolPreview(tool.fn);
  if (!Preview) return null;
  return (
    <PluginBoundary plugin={tool.fn} label={`${tool.fn} preview`}>
      <Preview tool={tool} onOpenView={onOpenView} />
    </PluginBoundary>
  );
}
