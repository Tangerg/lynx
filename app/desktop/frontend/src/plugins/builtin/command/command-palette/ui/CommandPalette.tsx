import type { IconName } from "@/ui";
import { Command } from "cmdk";
import { useMemo } from "react";
import { comboGlyph } from "@/lib/combo";
import { useT } from "@/lib/i18n";
import { Icon, Kbd } from "@/ui";
import { useCommands } from "@/plugins/sdk";
import { usePaletteStore } from "../../paletteStore";
import { useWhenContext } from "../../useWhenContext";
import { visibleCommands } from "../application/commandVisibility";
import { runPaletteCommand } from "../application/runPaletteCommand";

export function CommandPalette() {
  const t = useT();
  const open = usePaletteStore((state) => state.open);
  const setOpen = usePaletteStore((state) => state.setOpen);
  const commands = useCommands();
  const whenContext = useWhenContext();

  const visible = useMemo(() => visibleCommands(commands, whenContext), [commands, whenContext]);

  return (
    <Command.Dialog
      open={open}
      onOpenChange={setOpen}
      label={t("commandPalette.label")}
      className="fixed inset-0 z-50 flex items-start justify-center p-24 [&_[cmdk-overlay]]:fixed [&_[cmdk-overlay]]:inset-0 [&_[cmdk-overlay]]:bg-black/35"
    >
      <Command className="animate-rise-in relative z-[1] flex w-full max-w-[640px] flex-col overflow-hidden rounded-[14px] bg-canvas shadow-[var(--shadow-popover)]">
        <div className="flex items-center gap-2.5 px-4 py-3 text-fg-muted">
          <Icon name="search" size={15} />
          <Command.Input
            placeholder={t("commandPalette.placeholder")}
            className="flex-1 bg-transparent text-[15px] text-fg outline-none placeholder:text-fg-faint"
          />
          <Kbd>esc</Kbd>
        </div>
        <Command.List className="max-h-[400px] overflow-y-auto p-1.5">
          <Command.Empty className="px-3 py-6 text-center text-[12px] text-fg-faint">
            {t("commandPalette.empty")}
          </Command.Empty>
          {visible.map((command) => (
            <Command.Item
              key={command.id}
              value={[
                command.label,
                command.description ?? "",
                command.group ?? "",
                ...(command.keywords ?? []),
              ].join(" ")}
              onSelect={() => runPaletteCommand(command, () => setOpen(false))}
              className="flex h-9 items-center gap-2.5 rounded-md px-2.5 text-[13px] text-fg-soft hover:bg-fg/[0.06] aria-selected:bg-fg/[0.06] aria-selected:text-fg"
            >
              {command.icon && (
                <Icon
                  name={command.icon as IconName}
                  size={14}
                  className="shrink-0 text-fg-muted"
                />
              )}
              <div className="flex min-w-0 flex-1 flex-col">
                <div className="truncate font-medium">{command.label}</div>
                {command.description && (
                  <div className="truncate text-[11.5px] text-fg-faint">{command.description}</div>
                )}
              </div>
              {command.group && <span className="text-[11px] text-fg-faint">{command.group}</span>}
              {command.combo && <Kbd>{comboGlyph(command.combo)}</Kbd>}
            </Command.Item>
          ))}
        </Command.List>
      </Command>
    </Command.Dialog>
  );
}
