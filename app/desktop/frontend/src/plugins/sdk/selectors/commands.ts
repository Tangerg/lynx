// Command surface with real logic: the registered+declared merge (palette
// commands), the slash-command key pairing, and owner lookups for error
// attribution. Plain reads (a single command / shortcut by id) go through the
// generic substrate: `lookupExtensionByKey(COMMAND, id)`, `useExtensionPoint
// (SHORTCUT)`, etc.

import { useMemo } from "react";
import type { CommandSpec, ContributedCommand, SlashCommandSpec } from "../types";
import { COMMAND, SLASH_COMMAND } from "../kernelPoints";
import { usePluginStore } from "../registry";
import { runActivator, useDeclaredMerged } from "./_helpers";
import {
  lookupExtensionByKey,
  lookupExtensionOwner,
  useExtensionEntries,
  useExtensionPoint,
} from "./extensions";

// ---------------------------------------------------------------------------
// Palette commands (registered + declared merged)
// ---------------------------------------------------------------------------
//
// Registered wins on id collision, so once a plugin is activated its real
// CommandSpec replaces the contributes.commands placeholder transparently.

export function useCommands(): CommandSpec[] {
  const registered = useExtensionPoint(COMMAND);
  const declared = usePluginStore((s) => s.declaredCommands);
  return useDeclaredMerged(registered, declared, declaredToPlaceholder);
}

/** Owner plugin of a registered command — used for error attribution. */
export function lookupCommandOwner(id: string): string | undefined {
  return lookupExtensionOwner(COMMAND, id);
}

function declaredToPlaceholder(c: ContributedCommand, pluginName: string): CommandSpec {
  return {
    ...c,
    run: () => activateAndRun(pluginName, c.id),
  };
}

async function activateAndRun(pluginName: string, commandId: string): Promise<void> {
  await runActivator(pluginName);
  const real = lookupExtensionByKey(COMMAND, commandId);
  if (!real) {
    console.warn(`[plugin] ${pluginName} activated but did not register command ${commandId}`);
    return;
  }
  await real.run();
}

// ---------------------------------------------------------------------------
// Slash commands — the list pairs each spec with its trigger (the trigger
// lives in the map key, not on the spec, so the generic read can't surface it).
// ---------------------------------------------------------------------------

export function useSlashCommands(): Array<{ cmd: string; spec: SlashCommandSpec }> {
  const entries = useExtensionEntries(SLASH_COMMAND);
  return useMemo(() => entries.map((e) => ({ cmd: e.key, spec: e.item })), [entries]);
}

/** Owner plugin of a slash command — used for error attribution when one throws. */
export function lookupSlashCommandOwner(cmd: string): string | undefined {
  return lookupExtensionOwner(SLASH_COMMAND, cmd);
}
