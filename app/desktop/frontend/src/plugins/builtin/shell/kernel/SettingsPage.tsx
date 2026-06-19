// SettingsPage — the workspace view for app settings. Two-pane layout:
// a rail of plugin-registered panes on the left, the active pane on the
// right. Opens via Cmd+K → "View: Settings" or the sidebar-footer cog.

import type { IconName } from "@/components/common";
import * as Tabs from "@radix-ui/react-tabs";
import { useEffect, useState } from "react";
import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { useSettingsPanes } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

export function SettingsPage() {
  const t = useT();
  const panes = useSettingsPanes();
  // `selectedId` is the user's explicit choice. If they haven't picked
  // one (or their pick has since been unregistered), fall back to the
  // first pane via a derived value — no useEffect/setState loop. The initial
  // value honors a one-shot deep-link target (settingsPane, e.g. "providers"
  // from the keyless first-run onboarding), consumed + cleared on mount so a
  // later manual open starts at the first pane again.
  const setSettingsPane = useSessionStore((s) => s.setSettingsPane);
  const [selectedId, setSelectedId] = useState<string | undefined>(
    () => useSessionStore.getState().settingsPane ?? undefined,
  );
  useEffect(() => {
    if (useSessionStore.getState().settingsPane) setSettingsPane(null);
  }, [setSettingsPane]);
  const activeId = selectedId && panes.some((p) => p.id === selectedId) ? selectedId : panes[0]?.id;

  return (
    // Radix Tabs (vertical) → tablist/tab/tabpanel roles + arrow-key
    // navigation between panes for free. Controlled by the derived
    // `activeId` so the first-pane fallback stays a pure derivation.
    <Tabs.Root
      orientation="vertical"
      value={activeId}
      onValueChange={setSelectedId}
      className="grid h-full w-full grid-cols-[200px_1fr] overflow-hidden"
    >
      {/* Rail — the chat-tab strip already labels this view "Settings"
          and the highlighted item names the active pane, so neither a
          rail title nor a right-pane header is repeated here. */}
      <Tabs.List className="flex flex-col gap-0.5 px-2 py-3.5" aria-label={t("settings.title")}>
        {panes.map((p) => (
          <Tabs.Trigger
            key={p.id}
            value={p.id}
            // Hover === active background (settings-rail follows the
            // same rule as SessionRow / ProjectRow). Both states share
            // the same surface-3 lift + fg ink; nothing else
            // distinguishes them — the active pane reads as "selected"
            // via the chat-tab strip mirror in the topbar, not via a
            // second tone step inside the rail itself.
            className="flex items-center gap-2 rounded-md border-0 bg-transparent px-2.5 py-2 text-left text-[14px] text-fg-muted transition-colors duration-150 hover:bg-surface-3 hover:text-fg data-[state=active]:bg-surface-3 data-[state=active]:text-fg"
          >
            {p.icon && <Icon name={p.icon as IconName} size={15} />}
            <span>{t(p.label)}</span>
          </Tabs.Trigger>
        ))}
      </Tabs.List>
      <div className="min-h-0 min-w-0 overflow-y-auto bg-surface-2 px-5 py-4.5">
        {panes.map((p) => (
          <Tabs.Content key={p.id} value={p.id} className="outline-none">
            <PluginBoundary plugin={`settings:${p.id}`}>
              <p.component />
            </PluginBoundary>
          </Tabs.Content>
        ))}
      </div>
    </Tabs.Root>
  );
}
