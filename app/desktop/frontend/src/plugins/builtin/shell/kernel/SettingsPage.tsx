// SettingsPage — the workspace view for app settings. Two-pane layout:
// a grouped rail of plugin-registered panes on the left, the active pane on
// the right. Opens via Cmd+K → "View: Settings" or the sidebar-footer cog.

import type { IconName } from "@/components/common";
import * as Tabs from "@radix-ui/react-tabs";
import { useEffect, useState } from "react";
import { Icon, SectionLabel } from "@/components/common";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { useSettingsPanes } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

// Settings rail groups, in display order. A pane's `group` field places it
// here; anything with an unknown / missing group falls into the trailing
// bucket so nothing is ever dropped.
const GROUPS: { id: string; labelKey: string }[] = [
  { id: "general", labelKey: "settings.group.general" },
  { id: "models", labelKey: "settings.group.models" },
  { id: "agent", labelKey: "settings.group.agent" },
  { id: "integrations", labelKey: "settings.group.integrations" },
  { id: "advanced", labelKey: "settings.group.advanced" },
];
const FALLBACK_GROUP = "advanced";

export function SettingsPage() {
  const t = useT();
  const panes = useSettingsPanes();
  // `selectedId` is the user's explicit choice. If they haven't picked one (or
  // their pick has since been unregistered), fall back to the first pane via a
  // derived value — no useEffect/setState loop. The initial value honors a
  // one-shot deep-link target (settingsPane, e.g. "providers" from the keyless
  // first-run onboarding), consumed + cleared on mount.
  const setSettingsPane = useSessionStore((s) => s.setSettingsPane);
  const [selectedId, setSelectedId] = useState<string | undefined>(
    () => useSessionStore.getState().settingsPane ?? undefined,
  );
  useEffect(() => {
    // Consume the INITIAL deep-link (read into selectedId above), then keep
    // following LATER ones while this singleton view stays mounted. Settings is
    // a singleton workspace tab: a re-target (e.g. "configure MCP" from the
    // Tools view does setSettingsPane("mcp-servers") + openMainView) only
    // REFOCUSES Settings, never remounts it — so a mount-only consume would drop
    // the new target and leave settingsPane stale (a phantom jump on the next
    // fresh open). Subscribe so a deep-link applies while open.
    if (useSessionStore.getState().settingsPane) setSettingsPane(null);
    return useSessionStore.subscribe((s, prev) => {
      if (s.settingsPane && s.settingsPane !== prev.settingsPane) {
        setSelectedId(s.settingsPane);
        setSettingsPane(null);
      }
    });
  }, [setSettingsPane]);
  const activeId = selectedId && panes.some((p) => p.id === selectedId) ? selectedId : panes[0]?.id;

  // Bucket panes by group (they arrive order-sorted from the selector, so the
  // order is preserved within each group).
  const known = new Set(GROUPS.map((g) => g.id));
  const grouped = GROUPS.map((g) => ({
    ...g,
    panes: panes.filter((p) => (p.group && known.has(p.group) ? p.group : FALLBACK_GROUP) === g.id),
  })).filter((g) => g.panes.length > 0);

  return (
    // Radix Tabs (vertical) → tablist/tab/tabpanel roles + arrow-key navigation
    // for free. Controlled by the derived `activeId` so the first-pane fallback
    // stays a pure derivation.
    <Tabs.Root
      orientation="vertical"
      value={activeId}
      onValueChange={setSelectedId}
      className="grid h-full w-full grid-cols-[210px_1fr] overflow-hidden"
    >
      <Tabs.List
        className="flex flex-col gap-0.5 overflow-y-auto px-2 py-3"
        aria-label={t("settings.title")}
      >
        {grouped.map((g) => (
          <div key={g.id} className="flex flex-col gap-0.5">
            <SectionLabel>{t(g.labelKey)}</SectionLabel>
            {g.panes.map((p) => (
              <Tabs.Trigger
                key={p.id}
                value={p.id}
                className="flex items-center gap-2.5 rounded-md border-0 bg-transparent px-2.5 py-1.5 text-left text-[13px] text-fg-muted transition-colors duration-150 hover:bg-surface-3 hover:text-fg data-[state=active]:bg-surface-3 data-[state=active]:text-fg"
              >
                {p.icon && <Icon name={p.icon as IconName} size={15} className="shrink-0" />}
                <span className="truncate">{t(p.label)}</span>
              </Tabs.Trigger>
            ))}
          </div>
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
