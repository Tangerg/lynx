import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";

/**
 * The attribute a rendered block carries so anything outside the transcript can
 * find it in the DOM.
 *
 * An attribute rather than a registry of refs: the only question anyone asks of a
 * block from outside is "where is it on screen", and that answer lives in layout,
 * not in React state. A registry would have to be kept in sync with a list that
 * re-renders on every streamed token.
 */
export const BLOCK_ANCHOR_ATTR = "data-block-anchor";

/**
 * A stable identity for one rendered unit — used both as its React key and as its
 * DOM anchor.
 *
 * Identity, deliberately not position, wherever the block has one. HITL cards
 * hold per-interrupt local state (remembered decisions, edited arguments,
 * half-typed answers); keying them by index reuses the component instance when a
 * different interrupt lands at the same slot, which leaks one interrupt's draft
 * into the next. Only blocks with nothing better fall back to the index.
 */
export function renderUnitAnchor(messageId: string, unit: MessageRenderUnit): string {
  // A folded wave borrows the identity of what it holds, so opening the wave and then
  // rendering its members inline (which is what a pin does) does not remount them.
  if (unit.kind === "wave") {
    const first = unit.units[0];
    return first ? `${messageId}:w:${renderUnitAnchor(messageId, first)}` : `${messageId}:w:0`;
  }
  if (unit.kind === "toolGroup") return `${messageId}:g:${unit.tools[0]?.id ?? "0"}`;
  const { block, index } = unit;
  if (block.kind === "tool") return `${messageId}:t:${block.toolCallId}`;
  if ((block.kind === "approval" || block.kind === "question") && block.itemId) {
    return `${messageId}:i:${block.itemId}`;
  }
  return `${messageId}:b:${index}`;
}
