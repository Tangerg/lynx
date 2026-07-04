import { useEffect, useState } from "react";
import type { IconName } from "@/ui";
import { Icon, noDragClasses, VerticalTabs } from "@/ui";
import { AgentWindowControls } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import {
  clearWorkspaceSettingsPaneTarget,
  getWorkspaceSettingsPaneTarget,
  selectWorkspaceChat,
  useWorkspaceSettingsPaneTarget,
} from "@/plugins/builtin/workspace/public/navigation";
import { useSettingsPanes } from "@/plugins/sdk";

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
  const targetPane = useWorkspaceSettingsPaneTarget();
  const [selectedId, setSelectedId] = useState<string | undefined>(
    () => getWorkspaceSettingsPaneTarget() ?? undefined,
  );
  const [query, setQuery] = useState("");
  useEffect(() => {
    if (!targetPane) return;
    setSelectedId(targetPane);
    clearWorkspaceSettingsPaneTarget();
  }, [targetPane]);
  const normalizedQuery = query.trim().toLocaleLowerCase();

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
      }))
      .filter((item) =>
        normalizedQuery ? String(item.label).toLocaleLowerCase().includes(normalizedQuery) : true,
      ),
  })).filter((g) => g.items.length > 0);
  const visibleItems = grouped.flatMap((group) => group.items);
  const activeId =
    selectedId && visibleItems.some((p) => p.id === selectedId) ? selectedId : visibleItems[0]?.id;

  return (
    <VerticalTabs
      ariaLabel={t("settings.title")}
      groups={grouped}
      value={activeId}
      onValueChange={setSelectedId}
      sidebarHeader={
        <SettingsSidebarHeader
          query={query}
          onQueryChange={setQuery}
          searchPlaceholder={t("settings.searchPlaceholder")}
        />
      }
    />
  );
}

function SettingsSidebarHeader({
  query,
  onQueryChange,
  searchPlaceholder,
}: {
  query: string;
  onQueryChange: (value: string) => void;
  searchPlaceholder: string;
}) {
  const t = useT();
  return (
    <div className="pb-4">
      <AgentWindowControls />
      <button
        type="button"
        data-chrome-focus=""
        onClick={selectWorkspaceChat}
        className="mb-4 flex h-8 items-center gap-2 rounded-[8px] border-0 bg-transparent px-2 text-[13px] font-medium text-fg-muted transition-[background-color,color] duration-[120ms] hover:bg-fg/[0.045] hover:text-fg focus-visible:bg-fg/[0.06] focus-visible:outline-none"
      >
        <Icon name="arrow-left" size={15} strokeWidth={1.8} />
        <span>{t("settings.backToApp")}</span>
      </button>
      <label
        className={[
          "flex h-9 items-center gap-2 rounded-[8px] bg-canvas px-2.5 text-fg-muted",
          "shadow-[var(--shadow-border)] focus-within:text-fg focus-within:shadow-[var(--shadow-focus)]",
          noDragClasses,
        ].join(" ")}
      >
        <Icon name="search" size={15} strokeWidth={1.8} />
        <input
          value={query}
          onChange={(event) => onQueryChange(event.currentTarget.value)}
          placeholder={searchPlaceholder}
          className="min-w-0 flex-1 border-0 bg-transparent p-0 text-[13px] text-fg outline-none placeholder:text-fg-faint"
        />
      </label>
    </div>
  );
}
