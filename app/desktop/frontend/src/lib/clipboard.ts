// Clipboard domain service. The availability guard + permission-failure
// swallow is the invariant every call site needs (unfocused windows and
// non-secure contexts throw on write); only the success feedback differs —
// sites with their own inline "copied" state use copyText, sites without
// one use writeToClipboard for a toast confirmation.

import { toast } from "sonner";

/** Silent core: resolves false when the clipboard is unavailable or the
 *  write was rejected — never throws. */
export async function copyText(text: string): Promise<boolean> {
  if (!text || typeof navigator === "undefined" || !navigator.clipboard) return false;
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

export interface RichClipboardText {
  plainText: string;
  htmlText?: string;
}

/** Copy one semantic payload in both of the formats desktop editors understand.
 *  Older WebViews keep the exact plain-text fallback rather than losing the
 *  action because ClipboardItem is unavailable. */
export async function copyRichText({ plainText, htmlText }: RichClipboardText): Promise<boolean> {
  if (!plainText || typeof navigator === "undefined" || !navigator.clipboard) return false;
  const clipboard = navigator.clipboard;
  try {
    if (
      htmlText &&
      "write" in clipboard &&
      typeof clipboard.write === "function" &&
      typeof ClipboardItem !== "undefined"
    ) {
      await clipboard.write([
        new ClipboardItem({
          "text/plain": new Blob([plainText], { type: "text/plain" }),
          "text/html": new Blob([htmlText], { type: "text/html" }),
        }),
      ]);
    } else {
      await clipboard.writeText(plainText);
    }
    return true;
  } catch {
    return false;
  }
}

/** copyText + optional sonner confirmation. Success confirmations stay
 *  toast-only — they're feedback, not events worth re-reading in the
 *  notification feed. */
export async function writeToClipboard(
  text: string,
  options?: { successLabel?: string },
): Promise<boolean> {
  const ok = await copyText(text);
  if (ok && options?.successLabel) toast.success(options.successLabel);
  return ok;
}
