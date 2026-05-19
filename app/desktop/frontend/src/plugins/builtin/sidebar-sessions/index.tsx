// Built-in plugin: "Sessions" section in the expanded sidebar.
//
// Reads the sessions list + active id from app stores / TanStack Query —
// no props from the shell. Selection routes through `useUIStore.selectTab`.

import { SectionLabel } from "@/components/common";
import { SessionRow } from "@/components/sidebar/SessionRow";
import { useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function SessionsSection() {
  const { data: sessions = [] } = useSessions();
  const activeSessionId = useUIStore((s) => s.activeSessionId);
  const selectTab = useUIStore((s) => s.selectTab);

  return (
    <>
      <SectionLabel trailing={<span className="count">{sessions.length}</span>}>
        Sessions
      </SectionLabel>
      <div className="side-list">
        {sessions.map((s) => (
          <SessionRow
            key={s.id}
            session={s}
            active={s.id === activeSessionId}
            onSelect={selectTab}
          />
        ))}
      </div>
    </>
  );
}

export default definePlugin({
  name: "lyra.builtin.sidebar-sessions",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerSection({
      id: "sessions",
      order: 10,
      component: SessionsSection,
    });
  },
});
