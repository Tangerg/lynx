// Shared composer submit — used by the textarea Enter path, the send
// button, and any future trigger (palette command etc.). Owns slash
// routing + the always-clear-after-submit invariant.

import { buildInput, textInput, type InputImage, type UserInput } from "@/lib/agent/composerInput";
import {
  lookupExtensionByKey,
  lookupSlashCommandOwner,
  reportPluginError,
  SLASH_COMMAND,
} from "@/plugins/sdk";
import { useComposerStore } from "@/state/composerStore";

export interface SubmitDeps {
  /** Current textarea contents. */
  value: string;
  /** Wipe the textarea + its image attachments. Called once per successful submit. */
  clear: () => void;
  /** Forward the composed user input (text + inlined images) to the agent. */
  sendInput: (input: UserInput) => void;
  /** Image attachments to inline alongside the text (empty = text-only). */
  images: InputImage[];
}

/** Run the composer submit pipeline. Safe to call on empty text + no images. */
export function submitComposer({ value, clear, sendInput, images }: SubmitDeps): void {
  const text = value.trim();
  // An image-only send (a screenshot with no caption) is valid.
  if (!text && images.length === 0) return;

  // Record the submitted text for ↑/↓ recall (slash commands included — they're
  // worth re-running too). Image-only sends have no text to recall.
  if (text) useComposerStore.getState().pushHistory(text);

  // Slash routing applies only to a text command — an attached image isn't a
  // command argument. A "/cmd" still routes as the command (images dropped:
  // commands take plain string args, §slash).
  const slash = text ? parseSlash(text) : null;
  if (slash) {
    const spec = lookupExtensionByKey(SLASH_COMMAND, slash.cmd);
    if (spec?.run) {
      void Promise.resolve(
        spec.run({ args: slash.args, send: (t: string) => sendInput(textInput(t)) }),
      ).catch((err) => {
        console.error(`[plugin] command ${slash.cmd} threw:`, err);
        const owner = lookupSlashCommandOwner(slash.cmd) ?? "unknown";
        reportPluginError(owner, "command", err, `command: ${slash.cmd}`);
      });
      clear();
      return;
    }
  }
  sendInput(buildInput(text, images));
  clear();
}

// Parse the leading slash from the composer text. Splits on the first
// whitespace; everything after is treated as the command's args verbatim.
//
//   parseSlash("/lint src/foo.ts")  -> { cmd: "/lint", args: "src/foo.ts" }
//   parseSlash("/diff")             -> { cmd: "/diff", args: "" }
//   parseSlash("hello there")       -> null
export function parseSlash(text: string): { cmd: string; args: string } | null {
  if (!text.startsWith("/")) return null;
  const space = text.search(/\s/);
  if (space === -1) return { cmd: text, args: "" };
  return { cmd: text.slice(0, space), args: text.slice(space + 1) };
}
