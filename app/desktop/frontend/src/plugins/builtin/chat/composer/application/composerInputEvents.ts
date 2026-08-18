import { normalizeCombo } from "@/plugins/sdk/combo";
import { isLargePaste } from "../domain/largePaste";

export type ComposerPasteIntent =
  { kind: "images"; files: File[] } | { kind: "large-text"; text: string } | { kind: "native" };

export interface TransferItemLike {
  kind: string;
  type: string;
}

export function composerPasteIntent(files: File[], text: string): ComposerPasteIntent {
  if (files.length > 0) return { kind: "images", files };
  if (isLargePaste(text)) return { kind: "large-text", text };
  return { kind: "native" };
}

export function hasComposerImageTransferItems(
  items: Iterable<TransferItemLike> | ArrayLike<TransferItemLike> | null | undefined,
): boolean {
  return (
    !!items &&
    Array.from(items).some((item) => item.kind === "file" && item.type.startsWith("image/"))
  );
}

export function composerKeyBindingKey(
  event: Pick<KeyboardEvent, "metaKey" | "ctrlKey" | "altKey" | "shiftKey" | "key">,
): string {
  const parts: string[] = [];
  if (event.metaKey || event.ctrlKey) parts.push("mod");
  if (event.altKey) parts.push("alt");
  if (event.shiftKey) parts.push("shift");
  parts.push(event.key);
  return normalizeCombo(parts.join("+"));
}

export type ComposerCompositionKeyIntent = "active" | "committed-enter" | null;

export function composerCompositionKeyIntent(
  event: Pick<
    KeyboardEvent,
    "altKey" | "ctrlKey" | "isComposing" | "key" | "keyCode" | "metaKey" | "shiftKey"
  >,
  compositionActive: boolean,
  compositionCommitPending: boolean,
): ComposerCompositionKeyIntent {
  // WebKit keeps keyCode 229 on an IME-generated key event even when it has
  // already emitted compositionend and therefore reports isComposing=false.
  // This is an event fact, not a platform guess, so it also covers third-party
  // IMEs without a UA branch or a timing window.
  if (compositionActive || event.isComposing || event.keyCode === 229) return "active";

  // Other Chinese IMEs commit raw Latin text with compositionend followed by a
  // completely ordinary Enter. The controller carries that one lifecycle fact
  // into this classifier; modifiers remain explicit user shortcuts.
  return compositionCommitPending &&
    event.key === "Enter" &&
    !event.altKey &&
    !event.ctrlKey &&
    !event.metaKey &&
    !event.shiftKey
    ? "committed-enter"
    : null;
}
