import { useEffect, useState } from "react";
import type { IconName } from "@/ui";
import { Icon, SearchField, VerticalTabs } from "@/ui";
import { AgentSurfaceHeader } from "@/ui/agent";
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
      railHeader={
        <SettingsRailHeader
          query={query}
          onQueryChange={setQuery}
          searchPlaceholder={t("settings.searchPlaceholder")}
        />
      }
    />
  );
}

function SettingsRailHeader({
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
    <div className="flex flex-col">
      {/* Settings takes the whole window, so this is the ONLY chrome bar on the
          page: it clears the macOS traffic lights and it is what the user drags
          the window by. A hand-rolled spacer did the first job and not the
          second, which left the window immovable while settings was open. */}
      <AgentSurfaceHeader divider={false} windowCorner aria-hidden />
      <div className="px-4 pb-4">
        <button
          type="button"
          data-chrome-focus=""
          onClick={selectWorkspaceChat}
          className="mb-3 flex h-8 items-center gap-2 rounded-sm border-0 bg-transparent px-2 text-ui-lg font-medium text-fg-muted transition-[background-color,color] duration-[var(--dur-fast)] hover:bg-hover hover:text-fg focus-visible:bg-hover focus-visible:outline-none"
        >
          <Icon name="arrow-left" size={15} strokeWidth={1.8} />
          <span>{t("settings.backToApp")}</span>
        </button>
        <SearchField
          size="lg"
          value={query}
          onValueChange={onQueryChange}
          placeholder={searchPlaceholder}
          aria-label={searchPlaceholder}
        />
      </div>
    </div>
  );
}
