// Command surface with real logic: the registered+declared merge (palette
// commands), the slash-command key pairing, and owner lookups for error
// attribution. Plain reads (a single command / shortcut by id) go through the
// generic substrate: `lookupExtensionByKey(COMMAND, id)`, `useExtensionPoint
// (SHORTCUT)`, etc.

import { useMemo } from "react";
import type { CommandSpec, ContributedCommand, SlashCommandSpec } from "../types";
import { COMMAND, SLASH_COMMAND } from "../kernelPoints";
import { usePluginStore } from "../registry";
import { useResolvedContributions } from "./declaredContributions";
import {
  lookupExtensionByKey,
  lookupExtensionOwner,
  useExtensionEntries,
  useExtensionPoint,
} from "./extensions";
import { activatePlugin } from "./pluginActivation";

// Registered wins on id collision, so once a plugin is activated its real
// CommandSpec replaces the contributes.commands placeholder transparently.

export function useCommands(): CommandSpec[] {
  const registered = useExtensionPoint(COMMAND);
  const declared = usePluginStore((state) => state.declaredCommands);
  return useResolvedContributions(registered, declared, declaredToPlaceholder);
}

/** Owner plugin of a registered command — used for error attribution. */
export function lookupCommandOwner(id: string): string | undefined {
  return lookupExtensionOwner(COMMAND, id);
}

function declaredToPlaceholder(command: ContributedCommand, pluginName: string): CommandSpec {
  return {
    ...command,
    run: (...args) => activateAndRun(pluginName, command.id, args),
  };
}

async function activateAndRun(
  pluginName: string,
  commandId: string,
  args: unknown[],
): Promise<void> {
  await activatePlugin(pluginName);
  const real = lookupExtensionByKey(COMMAND, commandId);
  if (!real) {
    console.warn(`[plugin] ${pluginName} activated but did not register command ${commandId}`);
    return;
  }
  await real.run(...args);
}

/**
 * Run a command by id — the imperative cross-plugin invocation behind
 * `host.commands.execute`. Runs a registered command directly; activates a
 * lazy (declared-but-not-yet-loaded) command first, then runs it. Warns + is a
 * no-op when no command matches. Args are forwarded to the command's `run`.
 */
export function executeCommand(id: string, ...args: unknown[]): Promise<void> {
  const registered = lookupExtensionByKey(COMMAND, id);
  if (registered) return Promise.resolve(registered.run(...args));
  const declared = usePluginStore.getState().declaredCommands.get(id);
  if (declared) return activateAndRun(declared.pluginName, id, args);
  console.warn(`[plugin] commands.execute("${id}"): no command registered or declared`);
  return Promise.resolve();
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
