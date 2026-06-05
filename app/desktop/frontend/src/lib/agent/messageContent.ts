// Message → text helpers, shared by every surface that needs to
// project a `Message.blocks` array onto a flat string: the message-
// action copy menu, the conversation-export plugin, and the right-
// click context menu on the chat row. Kept in `lib/` (not a plugin
// folder) because two plugins + a kernel component already consume
// it — the underscore-prefixed plugin-internal location was the wrong
// home.

import { toast } from "sonner";
import type { Message } from "@/protocol/run/viewState";

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

/**
 * Async clipboard write, silent on permission failures so an unfocused
 * window or sandbox doesn't blow up the calling action. Pass
 * `successLabel` to surface a sonner success toast — the default is
 * silent so the helper stays usable from non-UI contexts (tests, batch
 * exports, etc).
 */
export async function writeToClipboard(
  text: string,
  options?: { successLabel?: string },
): Promise<boolean> {
  if (!text || typeof navigator === "undefined" || !navigator.clipboard) return false;
  try {
    await navigator.clipboard.writeText(text);
    // sonner is already in the main chunk (PluginToaster mounts it), so a
    // dynamic import here buys no code-splitting — just dispatch into the
    // existing instance.
    if (options?.successLabel) toast.success(options.successLabel);
    return true;
  } catch {
    return false;
  }
}
