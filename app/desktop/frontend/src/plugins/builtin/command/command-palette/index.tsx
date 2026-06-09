// Built-in plugin: Cmd+K command palette — now backed by `cmdk` (the
// same lib Linear / Vercel / Cursor / Anthropic Console use).
//
// cmdk gives us focus trap + portal + keyboard nav + built-in fuzzy
// search for free. We keep:
//   - the Mod+K shortcut + per-command `when` clause filtering
//   - the plugin-registry data source (useCommands())
//   - the error-isolated run() that reports failures into PluginsPane

import type { IconName } from "@/components/common";
import type { CommandSpec } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { Command } from "cmdk";
import { useMemo } from "react";
import { create } from "zustand";
import { Icon } from "@/components/common";
import {
  definePlugin,
  evalWhen,
  lookupCommandOwner,
  reportPluginError,
  useCommands,
} from "@/plugins/sdk";
import { useWhenContext } from "@/state/useWhenContext";

// ---------- open-state store ---------------------------------------------

interface PaletteState {
  open: boolean;
  setOpen: (open: boolean) => void;
  toggle: () => void;
}

const usePaletteStore = create<PaletteState>((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
  toggle: () => set((s) => ({ open: !s.open })),
}));

// ---------- palette UI ----------------------------------------------------

function CommandPalette() {
  const open = usePaletteStore((s) => s.open);
  const setOpen = usePaletteStore((s) => s.setOpen);
  const commands = useCommands();
  const whenCtx = useWhenContext();

  const visible = useMemo(
    () => commands.filter((c) => !c.when || evalWhen(c.when, whenCtx)),
    [commands, whenCtx],
  );

  const run = (cmd: CommandSpec) => {
    setOpen(false);
    void Promise.resolve(cmd.run()).catch((err) => {
      console.error(`[plugin] command ${cmd.id} threw:`, err);
      const owner = lookupCommandOwner(cmd.id) ?? "unknown";
      reportPluginError(owner, "command", err, `command: ${cmd.id}`);
    });
  };

  return (
    <Command.Dialog
      open={open}
      onOpenChange={setOpen}
      label="Command palette"
      className="fixed inset-0 z-50 flex items-start justify-center p-24 data-[state=open]:animate-in data-[state=closed]:animate-out [&_[cmdk-overlay]]:fixed [&_[cmdk-overlay]]:inset-0 [&_[cmdk-overlay]]:bg-black/40 [&_[cmdk-overlay]]:backdrop-blur-sm"
    >
      <Command className="relative z-[1] flex w-full max-w-[640px] flex-col overflow-hidden rounded-xl border border-line-soft bg-surface shadow-lg">
        <div className="flex items-center gap-2 border-b border-line-soft px-4 py-3 text-fg-faint">
          <Icon name="search" size={14} />
          <Command.Input
            placeholder="Type a command…"
            className="flex-1 bg-transparent text-[14px] text-fg outline-none placeholder:text-fg-faint"
          />
          <span className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-[10px] text-fg-faint">
            esc
          </span>
        </div>
        <Command.List className="max-h-[400px] overflow-y-auto p-1.5">
          <Command.Empty className="px-3 py-6 text-center text-[12px] text-fg-faint">
            No commands match
          </Command.Empty>
          {visible.map((cmd) => (
            <Command.Item
              key={cmd.id}
              value={[
                cmd.label,
                cmd.description ?? "",
                cmd.group ?? "",
                ...(cmd.keywords ?? []),
              ].join(" ")}
              onSelect={() => run(cmd)}
              className="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] text-fg-muted aria-selected:bg-surface-2 aria-selected:text-fg"
            >
              {cmd.icon && <Icon name={cmd.icon as IconName} size={14} />}
              <div className="flex min-w-0 flex-1 flex-col">
                <div className="truncate font-medium">{cmd.label}</div>
                {cmd.description && (
                  <div className="truncate text-[11.5px] text-fg-faint">{cmd.description}</div>
                )}
              </div>
              {cmd.group && (
                <span className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-[10px] text-fg-faint">
                  {cmd.group}
                </span>
              )}
              {cmd.shortcut && (
                <span className="ml-1 font-mono text-[11px] text-fg-faint">{cmd.shortcut}</span>
              )}
            </Command.Item>
          ))}
        </Command.List>
      </Command>
    </Command.Dialog>
  );
}

// ---------- plugin --------------------------------------------------------

export default definePlugin({
  name: "lyra.builtin.command-palette",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", {
      id: "command-palette",
      order: 10, // above settings (0), below toaster (100)
      component: CommandPalette,
    });

    host.extensions.contribute(SHORTCUT, {
      key: "Mod+K",
      description: "Open the command palette",
      // The palette swallows typing once open, so we want it to fire even
      // when an input is focused — that's the whole point of Cmd+K.
      allowInInputs: true,
      handler: (e) => {
        e.preventDefault();
        usePaletteStore.getState().toggle();
      },
    });

    // A command id for "open the palette" so other surfaces can trigger it
    // through the registry (e.g. the collapsed-rail Search button) instead of
    // reaching into this plugin's private open-state store.
    host.commands.register({
      id: "command.open",
      label: "Open command palette",
      icon: "command",
      group: "General",
      keywords: ["palette", "search", "command"],
      shortcut: "⌘K",
      run: () => usePaletteStore.getState().setOpen(true),
    });
  },
});
