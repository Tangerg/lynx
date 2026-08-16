// Contribution shapes that take more than the value to build: the layout point
// needs a stable id so re-registering a slot entry replaces rather than stacks,
// and the content-block point stores a renderer already narrowed to one kind.
// Free functions over an explicit ctx, not methods on an ambient host — the
// difference being that a plugin has to import what it uses.

import { createElement } from "react";
import type { Contributor } from "./definePlugin";
import { CONTENT_BLOCK, LAYOUT_SLOT } from "./kernelPoints";
import type { Disposable } from "./types/common";
import type { ContentBlock, ContentBlockKind, ContentBlockMap } from "./types/contentBlock";
import type { ContentBlockRenderer } from "./types/message";
import type { LayoutSlotSpec } from "./types/workspace";

export function contributeLayout(ctx: Contributor, slot: string, spec: LayoutSlotSpec): Disposable {
  return ctx.contribute(LAYOUT_SLOT, { slot, spec }, { id: `${slot}#${spec.id}` });
}

function isKind<K extends ContentBlockKind>(
  block: ContentBlock,
  kind: K,
): block is ContentBlockMap[K] {
  return block.kind === kind;
}

export function contributeContentBlock<K extends ContentBlockKind>(
  ctx: Contributor,
  kind: K,
  renderer: ContentBlockRenderer<K>,
): Disposable {
  return ctx.contribute(
    CONTENT_BLOCK,
    (block) => (isKind(block, kind) ? createElement(renderer, { block }) : null),
    { key: kind },
  );
}
