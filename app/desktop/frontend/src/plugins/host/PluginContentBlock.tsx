// Adapter for plugin-contributed content blocks.
//
// BlockRenderer hands unknown block kinds here; we look up the registered
// renderer and wrap it in a PluginBoundary so a buggy plugin renderer can't
// break the whole message.

import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import { PluginBoundary } from "./PluginBoundary";
import { CONTENT_BLOCK, useExtensionByKey } from "../sdk";

export function PluginContentBlock({ block }: { block: ContentBlock }) {
  const render = useExtensionByKey(CONTENT_BLOCK, block.kind);
  if (!render) return null;
  return (
    <PluginBoundary plugin={`content-block:${block.kind}`} label={`${block.kind} block`}>
      {render(block)}
    </PluginBoundary>
  );
}
