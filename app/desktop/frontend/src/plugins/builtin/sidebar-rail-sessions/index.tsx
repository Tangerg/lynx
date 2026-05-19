// Built-in plugin: the recent-sessions stack rendered in the middle of
// the collapsed sidebar. Reads sessions + active id from query/store
// directly — no props from the rail shell.

import { useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function RailSessions() {
  const { data: sessions = [] } = useSessions();
  const activeSessionId = useUIStore((s) => s.activeSessionId);
  const selectTab = useUIStore((s) => s.selectTab);
  const recent = sessions.slice(0, 5);

  return (
    <>
      <div className="rail-divider" />
      <div className="rail-sessions">
        {recent.map((s) => (
          <button
            key={s.id}
            className={`rail-session ${s.id === activeSessionId ? "active" : ""}`}
            title={s.title}
            onClick={() => selectTab(s.id)}
          >
            <span className="rail-session-glyph">{s.title.slice(0, 1).toUpperCase()}</span>
            {s.status === "running" && <span className="rail-pulse" />}
            {s.status === "waiting" && <span className="rail-pulse warn" />}
          </button>
        ))}
      </div>
    </>
  );
}

export default definePlugin({
  name: "lyra.builtin.sidebar-rail-sessions",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerRailItem({ id: "rail-sessions", order: 100, component: RailSessions });
  },
});
