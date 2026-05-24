// Shared composer submit — used by the textarea Enter path, the send
// button, and any future trigger (palette command etc.). Owns slash
// routing + the always-clear-after-submit invariant.

import { lookupSlashCommand, reportPluginError, usePluginStore } from "@/plugins/sdk";

export type SubmitDeps = {
  /** Current textarea contents. */
  value: string;
  /** Wipe the textarea. Called once per successful submit. */
  clear: () => void;
  /** Forward a plain user message to the agent. */
  sendText: (text: string) => void;
};

/** Run the composer submit pipeline. Safe to call on empty/whitespace text. */
export function submitComposer({ value, clear, sendText }: SubmitDeps): void {
  const text = value.trim();
  if (!text) return;

  const slash = parseSlash(text);
  if (slash) {
    const spec = lookupSlashCommand(slash.cmd);
    if (spec?.run) {
      void Promise.resolve(spec.run({ args: slash.args, send: sendText })).catch((err) => {
         
        console.error(`[plugin] command ${slash.cmd} threw:`, err);
        const owner =
          usePluginStore.getState().slashCommands.get(slash.cmd)?.pluginName ?? "unknown";
        reportPluginError(owner, "command", err, `command: ${slash.cmd}`);
      });
      clear();
      return;
    }
  }
  sendText(text);
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
