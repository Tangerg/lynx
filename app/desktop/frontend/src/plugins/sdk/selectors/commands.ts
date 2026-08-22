// Command surface with real logic: imperative execution plus slash-command key
// pairing and owner attribution. Plain reads (a single command / shortcut by id)
// go through the generic substrate: `lookupExtensionByKey(COMMAND, id)`,
// `useExtensionPoint(SHORTCUT)`, etc.

import { useMemo } from "react";
import type { SlashCommandSpec } from "../types";
import { COMMAND, SLASH_COMMAND } from "../kernelPoints";
import { lookupExtensionByKey, lookupExtensionOwner, useExtensionEntries } from "./extensions";

/**
 * Run a command by id — the imperative cross-plugin call. Warns and no-ops when
 * nothing matches; args forward to the command's `run`.
 */
export function executeCommand(id: string, ...args: unknown[]): Promise<void> {
  const command = lookupExtensionByKey(COMMAND, id);
  if (!command) {
    console.warn(`[plugin] commands.execute("${id}"): no command registered`);
    return Promise.resolve();
  }
  return Promise.resolve(command.run(...args));
}

// The slash-command trigger lives in the map key, not on the spec, so the
// generic read can't surface it — we pair key+spec into a list.

export function useSlashCommands(): Array<{ cmd: string; spec: SlashCommandSpec }> {
  const entries = useExtensionEntries(SLASH_COMMAND);
  return useMemo(() => entries.map((entry) => ({ cmd: entry.key, spec: entry.item })), [entries]);
}

/** Owner plugin of a slash command — used for error attribution when one throws. */
export function lookupSlashCommandOwner(cmd: string): string | undefined {
  return lookupExtensionOwner(SLASH_COMMAND, cmd);
}
