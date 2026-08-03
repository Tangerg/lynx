import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { renderUnitAnchor } from "./renderUnitAnchor";

export interface MessageOutlineEntry {
  /** The `BLOCK_ANCHOR_ATTR` of the prose block this heading lives in. */
  anchor: string;
  /** Position among that block's headings, in document order. */
  index: number;
  /** ATX depth, 1–6. Used for indent only; the rail does not renumber. */
  level: number;
  label: string;
}

const ATX = /^[ \t]{0,3}(#{1,6})[ \t]+(.+?)[ \t]*#*[ \t]*$/;
const FENCE = /^[ \t]{0,3}(?:```|~~~)/;

/** ATX headings of one markdown body, in document order.
 *
 *  Fenced code is skipped: a shell transcript full of `# comment` lines is the
 *  single most common thing an answer contains, and every one of them would
 *  otherwise enter the outline as a section.
 */
function headings(text: string): { level: number; label: string }[] {
  const found: { level: number; label: string }[] = [];
  let fenced = false;
  for (const line of text.split("\n")) {
    if (FENCE.test(line)) {
      fenced = !fenced;
      continue;
    }
    if (fenced) continue;
    const match = ATX.exec(line);
    if (match) found.push({ level: match[1]!.length, label: match[2]!.trim() });
  }
  return found;
}

/**
 * The answer's own table of contents.
 *
 * Headings of the message's prose, and nothing else. The rail used to enumerate
 * every render unit — each reasoning pass, each tool call — which on a working
 * turn is a hundred entries of "Thinking / grep / Thinking / read", i.e. a
 * transcript of the transcript. What a reader wants from a contents list is the
 * shape of the answer, and the only thing that knows that shape is the author's
 * own headings.
 */
export function messageOutline(message: Message): MessageOutlineEntry[] {
  const entries: MessageOutlineEntry[] = [];
  message.blocks.forEach((block, blockIndex) => {
    if (block.kind !== "text") return;
    const anchor = renderUnitAnchor(message.id, { kind: "block", block, index: blockIndex });
    headings(block.text).forEach(({ level, label }, index) => {
      entries.push({ anchor, index, level, label });
    });
  });
  return entries;
}
