// Sidebar primary navigation — the top-of-sidebar destinations that make the
// app's workspace-level features reachable in one click. Session-specific
// views live under the active session in the project tree, where their scope is
// visible. Renders above the Projects tree (order -10).

import type { IconName } from "@/components/common";
import { Icon, SectionLabel } from "@/components/common";
import { useCreateSession } from "@/plugins/builtin/agent/public/session";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SIDEBAR_SECTION } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";

interface Destination {
  id: string;
  icon: IconName;
  // i18n KEY for the view title. Doubles as the nav-row label and the opened
  // view's title — passed RAW (unresolved) to openMainView so it can be
  // re-translated live on a locale switch.
  titleKey: string;
}

// Reusable capabilities the agent draws on, independent of any single run.
const WORKSPACE_DESTINATIONS: Destination[] = [
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
      data-chrome-focus=""
      className={cn(
        "flex w-full items-center gap-2.5 rounded-md border-0 bg-transparent px-3 py-2 text-left",
        "font-sans text-[13px] font-medium text-fg transition-[background-color,transform] duration-75 active:scale-[0.99]",
        "hover:bg-fg/[0.045] hover:text-fg focus-visible:bg-fg/[0.065] focus-visible:text-fg focus-visible:outline-none",
        active && "bg-fg/[0.075] text-fg",
      )}
    >
      <Icon name={icon} size={15} className="shrink-0 text-fg" />
      <span className="flex-1 truncate">{label}</span>
    </button>
  );
}

function NavGroup({
  label,
  destinations,
  activeMainView,
}: {
  label: string;
  destinations: Destination[];
  activeMainView: string | null;
}) {
  const t = useT();
  return (
    <>
      <SectionLabel>{label}</SectionLabel>
      {destinations.map((d) => {
        const active = activeMainView === d.id;
        return (
          <NavRow
            key={d.id}
            icon={d.icon}
            label={t(d.titleKey)}
            active={active}
            onClick={() => {
              const store = useSessionStore.getState();
              if (active) {
                store.closeMainView(d.id);
              } else {
                store.openMainView({ id: d.id, title: d.titleKey, icon: d.icon });
              }
            }}
          />
        );
      })}
    </>
  );
}

function SidebarNav() {
  const t = useT();
  const createSession = useCreateSession();
  const activeMainView = useSessionStore((s) => s.activeMainView);

  return (
    <div className="flex flex-col gap-0.5">
      <NavRow icon="edit" label={t("sidebar.nav.newChat")} onClick={() => void createSession()} />
      <NavGroup
        label={t("sidebar.section.workspace")}
        destinations={WORKSPACE_DESTINATIONS}
        activeMainView={activeMainView}
      />
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
