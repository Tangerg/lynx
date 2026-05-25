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
