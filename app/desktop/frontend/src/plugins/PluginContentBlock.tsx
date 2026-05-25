// Adapter for plugin-contributed content blocks.
//
// PartRenderer hands unknown block kinds here; we look up the registered
// renderer and wrap it in a PluginBoundary so a buggy plugin renderer can't
// break the whole message.

import type { ContentBlock } from "@/protocol/agui/viewState";
import { PluginBoundary } from "./PluginBoundary";
import { useContentBlockRenderer } from "./sdk";

export function PluginContentBlock({ block }: { block: ContentBlock }) {
  const Renderer = useContentBlockRenderer(block.kind);
  if (!Renderer) return null;
  return (
    <PluginBoundary plugin={`content-block:${block.kind}`} label={`${block.kind} block`}>
      {/* Renderer's prop type is per-kind; storage widens to the union root.
          Cast the block to `any` here so React passes it through. */}
      {/* eslint-disable-next-line ts/no-explicit-any */}
      <Renderer block={block as any} />
    </PluginBoundary>
  );
}
