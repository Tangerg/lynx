// Shared composer submit use case — used by the textarea Enter path, the send
// button, and plugin key bindings. Owns slash routing + the
// always-clear-after-submit invariant; draft interpretation stays in the
// composer domain layer.

import {
  buildInput,
  textInput,
  type InputImage,
  type UserInput,
} from "@/plugins/builtin/chat/composer/public/input";
import type { PastedText } from "../domain/draft";
import { createComposerSendIntent } from "../domain/sendIntent";
import {
  lookupExtensionByKey,
  lookupSlashCommandOwner,
  reportPluginError,
  SLASH_COMMAND,
} from "@/plugins/sdk";

export interface SubmitDeps {
  /** Current textarea contents. */
  value: string;
  /** Wipe the textarea + its image attachments. Called once per successful submit. */
  clear: () => void;
  /** Forward the composed user input (text + inlined images) to the agent. */
  sendInput: (input: UserInput) => void;
  /** Image attachments to inline alongside the text (empty = text-only). */
  images: InputImage[];
  /** Large pasted text attachments staged on the active draft. */
  pastes: PastedText[];
  /** Record a submitted typed message for shell-style history recall. */
  recordHistory: (text: string) => void;
}

/** Run the composer submit pipeline. Safe to call on empty text + no images. */
export function submitComposer({
  value,
  clear,
  sendInput,
  images,
  pastes,
  recordHistory,
}: SubmitDeps): void {
  const intent = createComposerSendIntent({ value, images, pastes });
  // An image-only / paste-only send (a screenshot or a dropped blob with no
  // caption) is valid.
  if (!intent.shouldSend) return;

  // Record the submitted text for ↑/↓ recall (slash commands included — they're
  // worth re-running too). Image-/paste-only sends have no typed text to recall.
  if (intent.historyText) recordHistory(intent.historyText);

  // Slash routing applies only to a text command — an attached image / paste
  // isn't a command argument. A "/cmd" still routes as the command (attachments
  // dropped: commands take plain string args, §slash).
  const slash = intent.slash;
  if (slash) {
    const spec = lookupExtensionByKey(SLASH_COMMAND, slash.cmd);
    if (spec?.run) {
      void Promise.resolve(
        spec.run({ args: slash.args, send: (text: string) => sendInput(textInput(text)) }),
      ).catch((err) => {
        console.error(`[plugin] command ${slash.cmd} threw:`, err);
        const owner = lookupSlashCommandOwner(slash.cmd) ?? "unknown";
        reportPluginError(owner, "command", err, `command: ${slash.cmd}`);
      });
      clear();
      return;
    }
  }
  sendInput(buildInput(intent.body, images));
  clear();
}
