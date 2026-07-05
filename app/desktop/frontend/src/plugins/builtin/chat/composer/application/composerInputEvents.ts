import { normalizeCombo } from "@/plugins/sdk/registry";
import { isLargePaste } from "../domain/largePaste";

export type ComposerPasteIntent =
  { kind: "images"; files: File[] } | { kind: "large-text"; text: string } | { kind: "native" };

export function composerPasteIntent(files: File[], text: string): ComposerPasteIntent {
  if (files.length > 0) return { kind: "images", files };
  if (isLargePaste(text)) return { kind: "large-text", text };
  return { kind: "native" };
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
