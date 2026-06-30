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
import { comboGlyph } from "@/lib/combo";
import { Command } from "cmdk";
import { useMemo } from "react";
import { usePaletteStore } from "@/state/paletteStore";
import { Icon } from "@/components/common";
import { t, useT } from "@/lib/i18n";
import {
  definePlugin,
  evalWhen,
  lookupCommandOwner,
  reportPluginError,
  useCommands,
} from "@/plugins/sdk";
import { useWhenContext } from "@/state/useWhenContext";

function CommandPalette() {
  const t = useT();
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
      label={t("commandPalette.label")}
      className="fixed inset-0 z-50 flex items-start justify-center p-24 [&_[cmdk-overlay]]:fixed [&_[cmdk-overlay]]:inset-0 [&_[cmdk-overlay]]:bg-black/40 [&_[cmdk-overlay]]:backdrop-blur-sm"
    >
      <Command className="animate-rise-in relative z-[1] flex w-full max-w-[640px] flex-col overflow-hidden rounded-lg bg-surface shadow-lg">
        <div className="flex items-center gap-2 border-b border-line-soft px-4 py-3 text-fg-faint">
          <Icon name="search" size={14} />
          <Command.Input
            placeholder={t("commandPalette.placeholder")}
            className="flex-1 bg-transparent text-[14px] text-fg outline-none placeholder:text-fg-faint"
          />
          <span className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-[10px] text-fg-faint">
            esc
          </span>
        </div>
        <Command.List className="max-h-[400px] overflow-y-auto p-1.5">
          <Command.Empty className="px-3 py-6 text-center text-[12px] text-fg-faint">
            {t("commandPalette.empty")}
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
              {cmd.combo && (
                <span className="ml-1 font-mono text-[11px] text-fg-faint">
                  {comboGlyph(cmd.combo)}
                </span>
              )}
            </Command.Item>
          ))}
        </Command.List>
      </Command>
    </Command.Dialog>
  );
}

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
      label: t("command.openPalette"),
      icon: "command",
      group: "General",
      keywords: ["palette", "search", "command"],
      combo: "Mod+K",
      run: () => usePaletteStore.getState().setOpen(true),
    });
  },
});
