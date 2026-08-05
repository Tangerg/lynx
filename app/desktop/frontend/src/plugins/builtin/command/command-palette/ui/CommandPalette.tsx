import type { IconName } from "@/ui";
import type { ReactNode } from "react";
import { Command } from "cmdk";
import { useMemo, useState } from "react";
import { comboGlyph } from "@/lib/combo";
import { useT } from "@/lib/i18n";
import { FloatingSurface, Icon, Kbd, OptionRow } from "@/ui";
import { useCommands } from "@/plugins/sdk";
import { selectAgentSession, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { openWorkspaceViewInDock } from "@/plugins/builtin/workspace/public/navigation";
import { useContextDockCatalog } from "@/plugins/builtin/workspace/public/contextDockCatalog";
import { usePaletteStore } from "../../paletteStore";
import { useWhenContext } from "../../useWhenContext";
import { visibleCommands } from "../application/commandVisibility";
import { filterSessionsForPalette } from "../application/paletteSessions";
import { runPaletteCommand } from "../application/runPaletteCommand";

/**
 * One row, whatever it stands for.
 *
 * The icon box is rendered even when there is nothing to put in it: a command
 * without a glyph used to start its label where every other row's icon was, so
 * "Go back" sat a full gutter left of the rows above and below it.
 */
function PaletteRow({
  icon,
  label,
  detail,
  trailing,
}: {
  icon?: IconName;
  label: string;
  detail?: string;
  trailing?: ReactNode;
}) {
  return (
    <OptionRow layout="flex" size="lg">
      <span className="grid h-4 w-4 shrink-0 place-items-center text-fg-muted">
        {icon && <Icon name={icon} size="sm" />}
      </span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{label}</span>
        {detail && <span className="truncate text-ui-sm text-fg-faint">{detail}</span>}
      </span>
      {trailing}
    </OptionRow>
  );
}

const GROUP_HEADING = [
  "[&_[cmdk-group-heading]]:px-2",
  "[&_[cmdk-group-heading]]:pt-2.5",
  "[&_[cmdk-group-heading]]:pb-1",
  "[&_[cmdk-group-heading]]:text-ui-xs",
  "[&_[cmdk-group-heading]]:font-medium",
  "[&_[cmdk-group-heading]]:text-fg-faint",
].join(" ");

export function CommandPalette() {
  const t = useT();
  const open = usePaletteStore((state) => state.open);
  const setOpen = usePaletteStore((state) => state.setOpen);
  const commands = useCommands();
  const whenContext = useWhenContext();
  const { data: sessions } = useAgentSessions();
  const panelGroups = useContextDockCatalog();
  const [query, setQuery] = useState("");

  const visible = useMemo(() => visibleCommands(commands, whenContext), [commands, whenContext]);
  // Panels and sessions are offered only once the user types. An empty palette is
  // the seven things it can DO; the other two are large lists you arrive at by
  // name, and on an empty query they buried the commands under thirty rows.
  const searching = query.trim().length > 0;
  const sessionMatches = useMemo(
    () => filterSessionsForPalette(sessions ?? [], query),
    [sessions, query],
  );
  const panels = useMemo(
    () => (searching ? panelGroups.flatMap((group) => group.destinations) : []),
    [panelGroups, searching],
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
      // cmdk renders the overlay as a SIBLING of the content, so the arbitrary
      // descendant variant this used to carry (`[&_[cmdk-overlay]]:bg-scrim`)
      // matched nothing and the palette floated over undimmed live content for as
      // long as it has existed. The library has a prop for the overlay; use it.
      overlayClassName="fixed inset-0 z-50 bg-scrim"
      contentClassName="fixed inset-0 z-50 flex items-start justify-center p-24"
    >
      {/* The ring's floating surface rather than a fourth hand-spelled panel. It WRAPS
          cmdk's root instead of slotting into it: cmdk renders its own label element
          beside the children, so there is more than one child for `asChild` to slot onto
          and it throws. */}
      <FloatingSurface className="relative z-[1] flex w-full max-w-[560px] flex-col">
        <Command className="flex min-h-0 flex-col" loop>
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
          <Command.List className="max-h-[380px] overflow-y-auto px-1.5 pb-1.5">
            <Command.Empty className="px-3 py-6 text-center text-ui-md text-fg-faint">
              {t("commandPalette.empty")}
            </Command.Empty>
            {visible.map((command) => (
              <Command.Item
                key={command.id}
                // Search matches the words the user can see, so the keys resolve
                // before they reach the matcher. The group is still matchable even
                // though the row no longer prints it.
                value={[
                  t(command.label),
                  command.description ? t(command.description) : "",
                  command.group ? t(command.group) : "",
                  ...(command.keywords ?? []),
                ].join(" ")}
                onSelect={() => runPaletteCommand(command, close)}
                asChild
              >
                <PaletteRow
                  icon={command.icon as IconName | undefined}
                  label={t(command.label)}
                  detail={command.description ? t(command.description) : undefined}
                  trailing={command.combo && <Kbd>{comboGlyph(command.combo)}</Kbd>}
                />
              </Command.Item>
            ))}
            {panels.length > 0 && (
              <Command.Group heading={t("commandPalette.panels")} className={GROUP_HEADING}>
                {panels.map((panel) => (
                  <Command.Item
                    key={panel.viewId}
                    value={`panel ${t(panel.title)} ${panel.viewId}`}
                    onSelect={() => {
                      openWorkspaceViewInDock(panel.viewId);
                      close();
                    }}
                    asChild
                  >
                    <PaletteRow icon={panel.icon as IconName | undefined} label={t(panel.title)} />
                  </Command.Item>
                ))}
              </Command.Group>
            )}
            {sessionMatches.length > 0 && (
              <Command.Group heading={t("commandPalette.sessions")} className={GROUP_HEADING}>
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
                    <PaletteRow icon="chat" label={session.title} />
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
