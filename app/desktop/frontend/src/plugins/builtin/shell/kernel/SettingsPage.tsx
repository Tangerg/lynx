import type { ReactNode } from "react";
import { Suspense, useState } from "react";
import type { IconName } from "@/ui";
import { Button, Icon, SearchField, SkeletonList, VerticalTabs } from "@/ui";
import { AgentSurfaceHeader } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import {
  openWorkspaceSettingsPane,
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
  const [query, setQuery] = useState("");
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
          <SettingsPaneFrame
            title={t(p.label)}
            description={p.description ? t(p.description) : undefined}
          >
            <PluginBoundary plugin={`settings:${p.id}`}>
              <Suspense fallback={<SkeletonList count={4} label={t("common.loading")} />}>
                <p.component />
              </Suspense>
            </PluginBoundary>
          </SettingsPaneFrame>
        ),
      }))
      .filter((item) =>
        normalizedQuery ? String(item.label).toLocaleLowerCase().includes(normalizedQuery) : true,
      ),
  })).filter((g) => g.items.length > 0);
  const visibleItems = grouped.flatMap((group) => group.items);
  const activeId =
    targetPane && visibleItems.some((p) => p.id === targetPane) ? targetPane : visibleItems[0]?.id;

  return (
    <VerticalTabs
      ariaLabel={t("settings.title")}
      groups={grouped}
      value={activeId}
      onValueChange={(pane) => {
        if (pane) openWorkspaceSettingsPane(pane);
      }}
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

/**
 * Page framing belongs to the settings host, not to each plugin pane. A
 * contribution supplies its identity and body; the host gives every pane the
 * same title rhythm and keeps the heading visible if the plugin body fails.
 */
function SettingsPaneFrame({
  title,
  description,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section>
      <header>
        <h1 className="m-0 text-display-md font-semibold text-fg">{title}</h1>
        {description && (
          <p className="m-0 mt-1.5 max-w-[60ch] text-ui-md leading-6 text-fg-muted">
            {description}
          </p>
        )}
      </header>
      <div className="mt-6 pb-12">{children}</div>
    </section>
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
        <Button
          type="button"
          variant="ghost"
          size="md"
          press={false}
          data-chrome-focus=""
          onClick={selectWorkspaceChat}
          className="mb-3 flex h-8 items-center gap-2 rounded-sm border-0 bg-transparent px-2 text-ui-md font-medium text-fg-muted transition-[background-color,color] duration-[var(--dur-fast)] hover:bg-hover hover:text-fg focus-visible:bg-hover focus-visible:outline-none"
        >
          <Icon name="arrow-left" size="md" className="opacity-100" />
          <span>{t("settings.backToApp")}</span>
        </Button>
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
