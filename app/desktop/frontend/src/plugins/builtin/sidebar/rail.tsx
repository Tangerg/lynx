// Collapsed-sidebar rail: new session, sessions stack, bottom utilities.
// Three plugins because each maps to a different ordering bucket in the
// rail slot — keeping them in one file because they share no code with
// the expanded sidebar and only with each other.

import { Icon, IconButton } from "@/components/common";
import {
  useRecentWorkSessions,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { WORK_INDEX_ITEM } from "@/plugins/sdk/kernelPoints";

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
    host.extensions.contribute(WORK_INDEX_ITEM, {
      id: "new-session",
      scope: "global",
      placement: "rail",
      order: 10,
      component: NewSessionBtn,
    });
  },
});

function RailSessions() {
  const { activeSessionId, recentSessions } = useRecentWorkSessions(5);
  const actions = useWorkIndexActions();

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
            onClick={() => actions.selectSession(s.id)}
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
            {s.attention === "running" && (
              <span className="absolute top-1 right-1 h-2 w-2 rounded-full bg-accent shadow-[0_0_6px_var(--color-accent)] animate-pulse-dot" />
            )}
            {s.attention === "waiting" && (
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
    host.extensions.contribute(WORK_INDEX_ITEM, {
      id: "rail-sessions",
      scope: "session",
      placement: "rail",
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
    <IconButton
      variant="rail"
      title={t("sidebar.action.settings")}
      onClick={() => actions.openSettings(t("settings.title"))}
    >
      <Icon name="settings" size={16} />
    </IconButton>
  );
}

export const sidebarRailBottom = definePlugin({
  name: "lyra.builtin.sidebar-rail-bottom",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(WORK_INDEX_ITEM, {
      id: "rail-spacer",
      scope: "global",
      placement: "rail",
      order: 800,
      component: RailSpacer,
    });
    host.extensions.contribute(WORK_INDEX_ITEM, {
      id: "rail-context",
      scope: "session",
      placement: "rail",
      order: 900,
      component: RailContext,
    });
    host.extensions.contribute(WORK_INDEX_ITEM, {
      id: "rail-settings",
      scope: "global",
      placement: "rail",
      order: 910,
      component: RailSettings,
    });
  },
});
