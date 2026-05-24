// Collapsed-sidebar rail: top actions, sessions stack, bottom utilities.
// Three plugins because each maps to a different ordering bucket in the
// rail slot — keeping them in one file because they share no code with
// the expanded sidebar and only with each other.

import { Icon, IconButton } from "@/components/common";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

// ---- top actions --------------------------------------------------------

function NewSessionBtn() {
  const t = useT();
  return (
    <IconButton variant="rail-primary" title={t("sidebar.action.newSession")}>
      <Icon name="plus" size={16} />
    </IconButton>
  );
}

function SearchBtn() {
  const t = useT();
  return (
    <IconButton variant="rail" title={t("sidebar.action.searchHint")}>
      <Icon name="search" size={16} />
    </IconButton>
  );
}

export const sidebarRailActions = definePlugin({
  name: "lyra.builtin.sidebar-rail-actions",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerRailItem({ id: "new-session", order: 10, component: NewSessionBtn });
    host.sidebar.registerRailItem({ id: "search", order: 20, component: SearchBtn });
  },
});

// ---- sessions stack -----------------------------------------------------

function RailSessions() {
  const { data: sessions = [] } = useSessions();
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const selectTab = useSessionStore((s) => s.selectTab);
  const recent = sessions.slice(0, 5);

  return (
    <>
      {/* Hairline divider — kept as a tiny inline rule since the surface
          ladder doesn't carry separation at this 28px width. */}
      <div className="my-1.5 h-px w-7 bg-line" />
      <div className="flex w-full flex-col items-center gap-1">
        {recent.map((s) => (
          <button
            key={s.id}
            type="button"
            title={s.title}
            onClick={() => selectTab(s.id)}
            className={cn(
              // Dark: surface at rest, surface-2 on hover/active. Light
              // reads the same ladder but starts one tone deeper because
              // the panel itself is white.
              "relative grid h-9 w-9 place-items-center rounded-lg border-0 font-sans text-[13px] font-semibold cursor-pointer transition-[background,color,transform]",
              "bg-surface text-fg-muted light:bg-surface-2",
              "hover:bg-surface-2 hover:text-fg light:hover:bg-surface-3",
              s.id === activeSessionId &&
                "bg-surface-2 text-fg light:bg-surface-3 before:content-[''] before:absolute before:-left-2 before:top-2 before:bottom-2 before:w-[3px] before:rounded-full before:bg-accent",
            )}
          >
            <span className="font-sans text-[14px] font-semibold">
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
    host.sidebar.registerRailItem({ id: "rail-sessions", order: 100, component: RailSessions });
  },
});

// ---- bottom utilities ---------------------------------------------------

function RailSpacer() {
  return <div className="flex-1" />;
}

function RailTools() {
  const t = useT();
  return (
    <IconButton variant="rail" title={t("sidebar.action.tools")}>
      <Icon name="tool" size={16} />
    </IconButton>
  );
}

function RailSettings() {
  const t = useT();
  return (
    <IconButton variant="rail" title={t("sidebar.action.settings")}>
      <Icon name="settings" size={16} />
    </IconButton>
  );
}

function RailUser() {
  return (
    <div
      title="You · jdoe@longbridge-inc.com"
      className="mt-1 grid h-9 w-9 place-items-center rounded-full border-2 border-transparent bg-surface-2 font-sans text-[13px] font-semibold text-fg cursor-pointer transition-colors hover:border-accent"
    >
      J
    </div>
  );
}

export const sidebarRailBottom = definePlugin({
  name: "lyra.builtin.sidebar-rail-bottom",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerRailItem({ id: "rail-spacer", order: 800, component: RailSpacer });
    host.sidebar.registerRailItem({ id: "rail-tools", order: 900, component: RailTools });
    host.sidebar.registerRailItem({ id: "rail-settings", order: 910, component: RailSettings });
    host.sidebar.registerRailItem({ id: "rail-user", order: 920, component: RailUser });
  },
});
