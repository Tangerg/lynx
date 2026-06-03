// Tool preview router — looks up the renderer in the plugin registry.
// Every fn → component mapping lives in a plugin (built-ins included);
// there is no special-casing for "this is built-in".
//
// When no plugin previews the fn, falls back to a generic
// `ToolInspector` that shows raw args + result. That way third-party
// tools (or MCP tools we've never seen) still expand to something
// useful instead of an empty card.

import type { ToolCall } from "@/protocol/run/viewState";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { TOOL_PREVIEW, useExtensionByKey } from "@/plugins/sdk";
import { ToolInspector } from "./ToolInspector";

interface Props {
  tool: ToolCall;
  onOpenView: () => void;
}

export function ToolPreview({ tool, onOpenView }: Props) {
  const Preview = useExtensionByKey(TOOL_PREVIEW, tool.fn);
  if (!Preview) {
    return <ToolInspector tool={tool} />;
  }
  return (
    <PluginBoundary plugin={tool.fn} label={`${tool.fn} preview`}>
      <Preview tool={tool} onOpenView={onOpenView} />
    </PluginBoundary>
  );
}
