// Public message-text projection for surfaces that need to flatten an agent
// message: message actions, conversation export, and error recovery prompts.

import type { Message } from "@/plugins/sdk/types/agentSessionView";

/**
 * Best-effort plaintext extraction from a Message's content blocks. Only text +
 * reasoning (the prose-bearing kinds) contribute; tool / approval / question and
 * other UI-only blocks are dropped — their `text` is a card label (e.g. an
 * approval's "Run command"), not prose, so it must not leak into copied/exported
 * plaintext.
 */
export function flattenText(blocks: Message["blocks"]): string {
  return blocks
    .map((b) => (b.kind === "text" || b.kind === "reasoning" ? b.text : ""))
    .filter(Boolean)
    .join("\n\n");
}

/**
 * Markdown reconstruction — keeps the original markup so the consumer
 * sees the same headings / fences / lists they were rendered from.
 * Reasoning blocks render as italic block-quotes (LLM scratchpad). Other
 * non-text kinds are dropped — they're UI-only.
 */
export function flattenMarkdown(blocks: Message["blocks"]): string {
  const out: string[] = [];
  for (const b of blocks) {
    if (b.kind === "text" && b.text) {
      out.push(b.text);
    } else if (b.kind === "reasoning" && b.text) {
      const quoted = b.text
        .split("\n")
        .map((line) => `> *${line}*`)
        .join("\n");
      out.push(quoted);
    }
  }
  return out.join("\n\n");
}
