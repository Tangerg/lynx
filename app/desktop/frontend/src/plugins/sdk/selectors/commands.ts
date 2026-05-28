// Command surface — palette commands (registered + declared merged),
// slash commands, shortcuts. Includes the lazy-activation indirection
// for declared commands (a placeholder run() that triggers the plugin's
// setup, then re-dispatches to the now-registered real command).

import { useMemo } from "react";
import type { CommandSpec, ContributedCommand, ShortcutSpec, SlashCommandSpec } from "../types";
import { usePluginStore } from "../registry";
import { runActivator, useDeclaredMerged } from "./_helpers";

// ---------------------------------------------------------------------------
// Palette commands (registered + declared merged)
// ---------------------------------------------------------------------------
//
// Registered wins on id collision, so once a plugin is activated its real
// CommandSpec replaces the contributes.commands placeholder transparently.

export function useCommands(): CommandSpec[] {
  const registered = usePluginStore((s) => s.commands);
  const declared = usePluginStore((s) => s.declaredCommands);
  return useDeclaredMerged(registered, declared, declaredToPlaceholder);
}

/** Look up a registered command by id. */
export function lookupCommand(id: string): CommandSpec | undefined {
  return usePluginStore.getState().commands.get(id)?.value;
}

function declaredToPlaceholder(c: ContributedCommand, pluginName: string): CommandSpec {
  return {
    ...c,
    run: () => activateAndRun(pluginName, c.id),
  };
}

async function activateAndRun(pluginName: string, commandId: string): Promise<void> {
  await runActivator(pluginName);
  const real = lookupCommand(commandId);
  if (!real) {
    console.warn(`[plugin] ${pluginName} activated but did not register command ${commandId}`);
    return;
  }
  await real.run();
}

// ---------------------------------------------------------------------------
// Slash commands
// ---------------------------------------------------------------------------
//
// IMPORTANT — selector + useMemo split. Zustand's `useShallow` compares
// element-by-element with Object.is. Our selectors used to wrap each entry
// in a fresh `{ cmd, spec }` object every call, which never `Object.is`-
// equals the previous one — useShallow saw a "different" array on every
// render, useSyncExternalStore raised "result of getSnapshot should be
// cached", and we got "Maximum update depth exceeded".
//
// Pattern: the selector returns the raw Map (reference stable until a
// register/unregister mutates the registry). The component-side useMemo
// then derives whatever shape it needs, with the Map as a dep so it only
// recomputes when the underlying data actually changes.
export function useSlashCommands(): Array<{ cmd: string; spec: SlashCommandSpec }> {
  const map = usePluginStore((s) => s.slashCommands);
  return useMemo(
    () => Array.from(map.entries()).map(([cmd, owned]) => ({ cmd, spec: owned.value })),
    [map],
  );
}

/** Look up a slash command by exact key (including leading "/"). */
export function lookupSlashCommand(cmd: string): SlashCommandSpec | undefined {
  return usePluginStore.getState().slashCommands.get(cmd)?.value;
}

// ---------------------------------------------------------------------------
// Shortcuts
// ---------------------------------------------------------------------------

/** Look up a registered shortcut by canonical combo (after `normalizeCombo`). */
export function lookupShortcut(canonical: string): ShortcutSpec | undefined {
  return usePluginStore.getState().shortcuts.get(canonical)?.value;
}

/**
 * Every registered shortcut, in registration order. Used by the
 * cheat-sheet pane in Settings. ShortcutSpec has no `order` field, so
 * callers should sort by `description` (or whatever makes sense for
 * presentation) themselves.
 */
export function useShortcuts(): ShortcutSpec[] {
  const map = usePluginStore((s) => s.shortcuts);
  return useMemo(() => Array.from(map.values()).map((o) => o.value), [map]);
}
