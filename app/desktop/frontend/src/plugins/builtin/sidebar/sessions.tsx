import { DataView, SectionLabel } from "@/components/common";
import { SessionRow } from "@/components/sidebar/SessionRow";
import { useSessions } from "@/lib/queries";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";
import { sideListClasses } from "./styles";

function SessionsSection() {
  const t = useT();
  const { data: sessions, isLoading } = useSessions();
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const selectTab = useSessionStore((s) => s.selectTab);

  return (
    <>
      <SectionLabel
        trailing={
          <span className="ml-auto rounded-full bg-surface-2 px-1.5 py-px text-[10px] text-fg-muted">
            {sessions?.length ?? 0}
          </span>
        }
      >
        {t("sidebar.section.sessions")}
      </SectionLabel>
      <DataView
        items={sessions}
        isLoading={isLoading}
        skeletonCount={4}
        empty={{
          icon: "chat",
          title: "No sessions yet",
          sub: "Start a new conversation to see it here.",
          size: "compact",
        }}
      >
        {(items) => (
          <div className={sideListClasses}>
            {items.map((s) => (
              <SessionRow
                key={s.id}
                session={s}
                active={s.id === activeSessionId}
                onSelect={selectTab}
              />
            ))}
          </div>
        )}
      </DataView>
    </>
  );
}

export const sidebarSessions = definePlugin({
  name: "lyra.builtin.sidebar-sessions",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerSection({ id: "sessions", order: 10, component: SessionsSection });
  },
});
