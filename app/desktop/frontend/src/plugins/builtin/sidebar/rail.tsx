// Collapsed-sidebar rail: top actions, sessions stack, bottom utilities.
// Three plugins because each maps to a different ordering bucket in the
// rail slot — keeping them in one file because they share no code with
// the expanded sidebar and only with each other.

import { Icon, IconButton } from "@/components/common";
import { selectAgentSession, useCreateSession } from "@/plugins/builtin/agent/public/session";
import { useRecentWorkSessions } from "@/plugins/builtin/navigation/public/workIndex";
import { openWorkspaceView } from "@/plugins/builtin/workspace/public/navigation";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SIDEBAR_RAIL_ITEM } from "@/plugins/sdk/kernelPoints";

// Open a workspace view in the main pane (mirrors the expanded sidebar's
// footer gear / Tools entry). getState() in the handler — no subscription.
const openView = (id: string, title: string, icon: string) =>
  openWorkspaceView({ id, title, icon });

function NewSessionBtn() {
  const t = useT();
  const createSession = useCreateSession();
  return (
    <IconButton
      variant="rail-primary"
      title={t("sidebar.action.newSession")}
      onClick={() => void createSession()}
    >
      <Icon name="plus" size={16} />
    </IconButton>
  );
}

function SearchBtn() {
  const t = useT();
  return (
    <IconButton
      variant="rail"
      title={t("sidebar.action.searchHint")}
      onClick={() => openView("search", "workspace.view.title.search", "search")}
    >
      <Icon name="search" size={16} />
    </IconButton>
  );
}

export const sidebarRailActions = definePlugin({
  name: "lyra.builtin.sidebar-rail-actions",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SIDEBAR_RAIL_ITEM, {
      id: "new-session",
      order: 10,
      component: NewSessionBtn,
    });
    host.extensions.contribute(SIDEBAR_RAIL_ITEM, {
      id: "search",
      order: 20,
      component: SearchBtn,
    });
  },
});

function RailSessions() {
  const { activeSessionId, recentSessions } = useRecentWorkSessions(5);

  return (
    <>
      {/* Hairline divider — kept as a tiny inline rule since the surface
          ladder doesn't carry separation at this 28px width. */}
      <div className="my-1.5 h-px w-7 bg-line" />
      <div className="flex w-full flex-col items-center gap-1">
        {recentSessions.map((s) => (
          <button
            key={s.id}
            type="button"
            title={s.title}
            onClick={() => selectAgentSession(s.id)}
            className={cn(
              "relative grid h-10 w-10 place-items-center rounded-md border-0 font-sans text-[13px] font-medium transition-[background-color,color,transform] duration-75",
              "text-fg-muted hover:bg-fg/[0.02] hover:text-fg",
              s.id === activeSessionId &&
                "bg-fg/[0.03] text-fg before:content-[''] before:absolute before:left-0 before:inset-y-0 before:w-[2px] before:bg-accent before:rounded-full",
            )}
          >
            <span className="font-sans text-[14px] font-medium">
              {s.title.slice(0, 1).toUpperCase()}
            </span>
            {s.status === "running" && (
              <span className="absolute top-1 right-1 h-2 w-2 rounded-full bg-accent shadow-[0_0_6px_var(--color-accent)] animate-pulse-dot" />
            )}
            {s.status === "waiting" && (
              <span className="absolute top-1 right-1 h-2 w-2 rounded-full bg-warning shadow-[0_0_6px_var(--color-warning)]" />
            )}
          </button>
        ))}
      </div>
    </>
  );
}

export const sidebarRailSessions = definePlugin({
  name: "lyra.builtin.sidebar-rail-sessions",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SIDEBAR_RAIL_ITEM, {
      id: "rail-sessions",
      order: 100,
      component: RailSessions,
    });
  },
});

function RailSpacer() {
  return <div className="flex-1" />;
}

function RailTools() {
  const t = useT();
  return (
    <IconButton
      variant="rail"
      title={t("sidebar.action.tools")}
      onClick={() => openView("tools", t("sidebar.action.tools"), "tool")}
    >
      <Icon name="tool" size={16} />
    </IconButton>
  );
}

function RailSettings() {
  const t = useT();
  return (
    <IconButton
      variant="rail"
      title={t("sidebar.action.settings")}
      onClick={() => openView("settings", t("settings.title"), "settings")}
    >
      <Icon name="settings" size={16} />
    </IconButton>
  );
}

export const sidebarRailBottom = definePlugin({
  name: "lyra.builtin.sidebar-rail-bottom",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SIDEBAR_RAIL_ITEM, {
      id: "rail-spacer",
      order: 800,
      component: RailSpacer,
    });
    host.extensions.contribute(SIDEBAR_RAIL_ITEM, {
      id: "rail-tools",
      order: 900,
      component: RailTools,
    });
    host.extensions.contribute(SIDEBAR_RAIL_ITEM, {
      id: "rail-settings",
      order: 910,
      component: RailSettings,
    });
  },
});
