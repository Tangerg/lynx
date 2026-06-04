import { useMemo } from "react";
import { DataView, SectionLabel } from "@/components/common";
import { SessionRow } from "@/components/sidebar/SessionRow";
import { useT } from "@/lib/i18n";
import { useDeleteSession } from "@/lib/agent/useDeleteSession";
import { useSessions } from "@/lib/data/queries";
import { definePlugin } from "@/plugins/sdk";
import { SIDEBAR_SECTION } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";
import { sideListClasses } from "./styles";

function SessionsSection() {
  const t = useT();
  const { data, isLoading, isError } = useSessions();
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const draftIds = useSessionStore((s) => s.draftSessionIds);
  const selectTab = useSessionStore((s) => s.selectTab);
  const deleteSession = useDeleteSession();
  // Hide draft sessions (created but not yet sent to) — they graduate into
  // the list on first message.
  const sessions = useMemo(
    () => (data ? data.filter((s) => !draftIds.has(s.id)) : data),
    [data, draftIds],
  );

  return (
    <>
      <SectionLabel
        trailing={
          // No count pill until the first fetch settles — otherwise it flashes
          // "0" next to the loading skeletons.
          isLoading ? undefined : (
            <span className="ml-auto rounded-full bg-surface-2 px-1.5 py-px text-[10px] text-fg-muted">
              {sessions?.length ?? 0}
            </span>
          )
        }
      >
        {t("sidebar.section.sessions")}
      </SectionLabel>
      <DataView
        items={sessions}
        isLoading={isLoading}
        isError={isError}
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
                onDelete={deleteSession}
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
    host.extensions.contribute(SIDEBAR_SECTION, {
      id: "sessions",
      order: 10,
      component: SessionsSection,
    });
  },
});
