import type { IconName } from "@/ui";
import { Command } from "cmdk";
import { useMemo, useState } from "react";
import { comboGlyph } from "@/lib/combo";
import { useT } from "@/lib/i18n";
import { FloatingSurface, Icon, Kbd, OptionRow } from "@/ui";
import { useCommands } from "@/plugins/sdk";
import { selectAgentSession, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { usePaletteStore } from "../../paletteStore";
import { useWhenContext } from "../../useWhenContext";
import { visibleCommands } from "../application/commandVisibility";
import { filterSessionsForPalette } from "../application/paletteSessions";
import { runPaletteCommand } from "../application/runPaletteCommand";

export function CommandPalette() {
  const t = useT();
  const open = usePaletteStore((state) => state.open);
  const setOpen = usePaletteStore((state) => state.setOpen);
  const commands = useCommands();
  const whenContext = useWhenContext();
  const { data: sessions } = useAgentSessions();
  const [query, setQuery] = useState("");

  const visible = useMemo(() => visibleCommands(commands, whenContext), [commands, whenContext]);
  // Session jump is offered only once the user types (the palette stays
  // commands-only on an empty query). cmdk still ranks the rendered items by the
  // same query, so pre-filtering here just bounds the DOM, not the matching.
  const sessionMatches = useMemo(
    () => filterSessionsForPalette(sessions ?? [], query),
    [sessions, query],
  );

  const close = () => {
    setOpen(false);
    setQuery("");
  };

  return (
    <Command.Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) setQuery("");
      }}
      label={t("commandPalette.label")}
      className="fixed inset-0 z-50 flex items-start justify-center p-24 [&_[cmdk-overlay]]:fixed [&_[cmdk-overlay]]:inset-0 [&_[cmdk-overlay]]:bg-scrim"
    >
      {/* The ring's floating surface rather than a fourth hand-spelled panel. It WRAPS
          cmdk's root instead of slotting into it: cmdk renders its own label element
          beside the children, so there is more than one child for `asChild` to slot onto
          and it throws. */}
      <FloatingSurface className="relative z-[1] flex w-full max-w-[640px] flex-col">
        <Command className="flex min-h-0 flex-col">
          <div className="flex items-center gap-2.5 px-4 py-3 text-fg-muted">
            <Icon name="search" size="md" />
            <Command.Input
              value={query}
              onValueChange={setQuery}
              placeholder={t("commandPalette.placeholder")}
              className="flex-1 bg-transparent text-ui-md text-fg outline-none placeholder:text-fg-faint"
            />
            <Kbd>esc</Kbd>
          </div>
          <Command.List className="max-h-[400px] overflow-y-auto p-1.5">
            <Command.Empty className="px-3 py-6 text-center text-ui-md text-fg-faint">
              {t("commandPalette.empty")}
            </Command.Empty>
            {visible.map((command) => (
              <Command.Item
                key={command.id}
                // Search matches the words the user can see, so the keys resolve
                // before they reach the matcher.
                value={[
                  t(command.label),
                  command.description ? t(command.description) : "",
                  command.group ? t(command.group) : "",
                  ...(command.keywords ?? []),
                ].join(" ")}
                onSelect={() => runPaletteCommand(command, close)}
                asChild
              >
                <OptionRow layout="flex" size="lg">
                  {command.icon && (
                    <Icon name={command.icon as IconName} size="sm" className="shrink-0 text-fg" />
                  )}
                  <div className="flex min-w-0 flex-1 flex-col">
                    <div className="truncate font-medium">{t(command.label)}</div>
                    {command.description && (
                      <div className="truncate text-ui-sm text-fg-faint">
                        {t(command.description)}
                      </div>
                    )}
                  </div>
                  {command.group && (
                    <span className="text-ui-sm text-fg-faint">{t(command.group)}</span>
                  )}
                  {command.combo && <Kbd>{comboGlyph(command.combo)}</Kbd>}
                </OptionRow>
              </Command.Item>
            ))}
            {sessionMatches.length > 0 && (
              <Command.Group
                heading={t("commandPalette.sessions")}
                className="[&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-ui-sm [&_[cmdk-group-heading]]:text-fg-faint"
              >
                {sessionMatches.map((session) => (
                  <Command.Item
                    key={session.id}
                    value={`session ${session.title} ${session.id}`}
                    onSelect={() => {
                      selectAgentSession(session.id);
                      close();
                    }}
                    asChild
                  >
                    <OptionRow layout="flex" size="lg">
                      <Icon name="chat" size="sm" className="shrink-0 text-fg-faint" />
                      <div className="min-w-0 flex-1 truncate font-medium">{session.title}</div>
                    </OptionRow>
                  </Command.Item>
                ))}
              </Command.Group>
            )}
          </Command.List>
        </Command>
      </FloatingSurface>
    </Command.Dialog>
  );
}
