import { useEffect, useState } from "react";
import type { IconName } from "@/components/common";
import { VerticalTabs } from "@/components/common";
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
    label: t(g.labelKey),
    items: panes
      .filter((p) => (p.group && known.has(p.group) ? p.group : FALLBACK_GROUP) === g.id)
      .map((p) => ({
        id: p.id,
        label: t(p.label),
        icon: p.icon as IconName | undefined,
        content: (
          <PluginBoundary plugin={`settings:${p.id}`}>
            <p.component />
          </PluginBoundary>
        ),
      })),
  })).filter((g) => g.items.length > 0);

  return (
    <VerticalTabs
      ariaLabel={t("settings.title")}
      groups={grouped}
      value={activeId}
      onValueChange={setSelectedId}
    />
  );
}
