// Public message-text projection for surfaces that need to flatten an agent
// message: message actions, conversation export, and error recovery prompts.

import type { Message } from "@/plugins/builtin/agent/public/viewState";

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
 * Reasoning blocks render as italic block-quotes (LLM scratchpad);
 * code blocks become fenced blocks with their language tag. Other
 * non-text kinds (tool / approval / search / plan / checkpoint) are
 * dropped — they're UI-only.
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
    } else if (b.kind === "code" && b.text) {
      out.push(`\`\`\`${b.lang}\n${b.text}\n\`\`\``);
    }
  }
  return out.join("\n\n");
}

/**
 * Code-only extraction — every fenced code block in source order. Useful
 * when the user wants to paste just the generated code into their editor
 * without the prose around it. Returns "" if the message has no code.
 */
export function flattenCode(blocks: Message["blocks"]): string {
  const out: string[] = [];
  for (const b of blocks) {
    if (b.kind === "code" && b.text) out.push(b.text);
  }
  return out.join("\n\n");
}
