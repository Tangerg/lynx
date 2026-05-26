// Shared bits for the three message-action button plugins (copy /
// edit / regenerate) — same hover affordance, same Tailwind chrome,
// same best-effort text extraction. Underscore prefix marks this file
// as plugin-internal: builtin/index.ts shouldn't re-export it.

import type { Message } from "@/protocol/agui/viewState";

/** Shared Tailwind class string for the hover-revealed icon buttons in
 *  the message header (copy / edit / regenerate). */
export const ACTION_BTN_CLASSES =
  "inline-flex h-5 w-5 items-center justify-center rounded border-0 bg-transparent text-fg-faint cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg";

/**
 * Best-effort plaintext extraction from a Message's content blocks. Only
 * blocks that carry a `text` field contribute (tool / approval / search
 * blocks fall through — they don't make sense as plain text anyway).
 */
export function flattenText(blocks: Message["blocks"]): string {
  return blocks
    .map((b) => ("text" in b ? ((b as { text?: string }).text ?? "") : ""))
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
