// Collapsed-sidebar rail: new session, sessions stack, bottom utilities.
// Three plugins because each maps to a different ordering bucket in the
// rail slot — keeping them in one file because they share no code with
// the expanded sidebar and only with each other.

import { Icon, IconButton } from "@/ui";
import {
  contributeWorkIndexItem,
  useRecentWorkSessions,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";

function NewSessionBtn() {
  const t = useT();
  const actions = useWorkIndexActions();
  return (
    <IconButton
      variant="rail-primary"
      title={t("sidebar.action.newSession")}
      onClick={actions.createSession}
    >
      <Icon name="plus" size={16} />
    </IconButton>
  );
}

export const sidebarRailNewSession = definePlugin({
  name: "lyra.builtin.sidebar-rail-new-session",
  version: "1.0.0",
  setup({ host }) {
    contributeWorkIndexItem(host, {
      id: "rail-new-session",
      scope: "global",
      variant: "rail",
      order: 10,
      component: NewSessionBtn,
    });
  },
});

function RailSessions() {
  const { activeSessionId, recentSessions } = useRecentWorkSessions(5);
  const actions = useWorkIndexActions();

  return (
    // Separation from the top rail actions is whitespace, not a rule — the
    // Codex rail carries no dividers.
    <div className="mt-1.5 flex w-full flex-col items-center gap-1">
      {recentSessions.map((s) => (
        <button
          key={s.id}
          type="button"
          title={s.title}
          onClick={() => actions.selectSession(s.id)}
          className={cn(
            "relative grid h-10 w-10 place-items-center rounded-[9px] border-0 bg-transparent font-sans text-[14px] font-medium transition-[background-color,color,scale] duration-[120ms] ease-out active:scale-[0.96]",
            "text-fg-muted hover:bg-fg/[0.045] hover:text-fg",
            s.id === activeSessionId && "bg-fg/[0.06] text-fg",
          )}
        >
          <span>{s.title.slice(0, 1).toUpperCase()}</span>
          {s.attention === "running" && (
            <span className="absolute top-1 right-1 h-2 w-2 rounded-full bg-accent animate-pulse-dot" />
          )}
          {s.attention === "waiting" && (
            <span className="absolute top-1 right-1 h-2 w-2 rounded-full bg-warning" />
          )}
        </button>
      ))}
    </div>
  );
}

export const sidebarRailSessions = definePlugin({
  name: "lyra.builtin.sidebar-rail-sessions",
  version: "1.0.0",
  setup({ host }) {
    contributeWorkIndexItem(host, {
      id: "rail-sessions",
      scope: "session",
      variant: "rail",
      order: 100,
      component: RailSessions,
    });
  },
});

function RailSpacer() {
  return <div className="flex-1" />;
}

function RailContext() {
  const t = useT();
  const actions = useWorkIndexActions();
  return (
    <IconButton
      variant="rail"
      title={t("workspace.view.title.context")}
      onClick={actions.openContextDock}
    >
      <Icon name="panel-r" size={16} />
    </IconButton>
  );
}

function RailSettings() {
  const t = useT();
  const actions = useWorkIndexActions();
  return (
    <IconButton variant="rail" title={t("sidebar.action.settings")} onClick={actions.openSettings}>
      <Icon name="settings" size={16} />
    </IconButton>
  );
}

export const sidebarRailBottom = definePlugin({
  name: "lyra.builtin.sidebar-rail-bottom",
  version: "1.0.0",
  setup({ host }) {
    contributeWorkIndexItem(host, {
      id: "rail-spacer",
      scope: "global",
      variant: "rail",
      order: 800,
      component: RailSpacer,
    });
    contributeWorkIndexItem(host, {
      id: "rail-context",
      scope: "session",
      variant: "rail",
      order: 900,
      component: RailContext,
    });
    contributeWorkIndexItem(host, {
      id: "rail-settings",
      scope: "global",
      variant: "rail",
      order: 910,
      component: RailSettings,
    });
  },
});
