// Sidebar primary navigation — the top-of-sidebar destinations that make the
// app's features reachable in one click (the "where do I find X?" fix). A "New
// chat" action plus a labeled Workspace group of the main feature views. Each
// workspace row opens its view in the main pane (openMainView) and lights up
// while that view is active. Renders above the Projects tree (order -10).

import type { IconName } from "@/components/common";
import { Icon, SectionLabel } from "@/components/common";
import { useCreateSession } from "@/lib/agent/useCreateSession";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SIDEBAR_SECTION } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";

// Curated feature destinations surfaced as one-click entries. Each `id` is a
// registered WORKSPACE_VIEW; `titleKey` doubles as the row label and the opened
// tab's title so the two never drift.
const WORKSPACE_DESTINATIONS: { id: string; icon: IconName; titleKey: string }[] = [
  { id: "codebase", icon: "folder-search", titleKey: "codebase.title" },
  { id: "skills", icon: "spark", titleKey: "skills.title" },
  { id: "recipes", icon: "book", titleKey: "recipes.title" },
  { id: "tools", icon: "tool", titleKey: "tools.title" },
  { id: "memory", icon: "filetext", titleKey: "memory.title" },
];

function NavRow({
  icon,
  label,
  active,
  onClick,
}: {
  icon: IconName;
  label: string;
  active?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-2.5 rounded-lg border-0 bg-transparent px-2.5 py-1.5 text-left",
        "font-sans text-[13px] text-fg-muted transition-colors active:scale-[0.99]",
        "hover:bg-surface-2 hover:text-fg light:hover:bg-surface-3",
        active && "bg-surface-2 text-fg light:bg-surface-3",
      )}
    >
      <Icon name={icon} size={15} className="shrink-0 text-fg-faint" />
      <span className="flex-1 truncate">{label}</span>
    </button>
  );
}

function SidebarNav() {
  const t = useT();
  const createSession = useCreateSession();
  const activeMainView = useSessionStore((s) => s.activeMainView);

  return (
    <div className="flex flex-col gap-0.5 px-1">
      <NavRow icon="edit" label={t("sidebar.nav.newChat")} onClick={() => void createSession()} />
      <SectionLabel>{t("sidebar.section.workspace")}</SectionLabel>
      {WORKSPACE_DESTINATIONS.map((d) => (
        <NavRow
          key={d.id}
          icon={d.icon}
          label={t(d.titleKey)}
          active={activeMainView === d.id}
          onClick={() =>
            useSessionStore
              .getState()
              .openMainView({ id: d.id, title: t(d.titleKey), icon: d.icon })
          }
        />
      ))}
    </div>
  );
}

export const sidebarNav = definePlugin({
  name: "lyra.builtin.sidebar-nav",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SIDEBAR_SECTION, {
      id: "nav",
      order: -10,
      component: SidebarNav,
    });
  },
});
