// Built-in plugins that fill the sidebar's slots — brand mark, search,
// projects/sessions sections, footer user-card, and the rail (collapsed)
// equivalents.
//
// Each piece is still its own plugin so a re-skinned build can swap any
// single contribution without forking the others. They live together
// because they're individually small.

import { EmptyState, Icon, IconButton, SectionLabel, SkeletonList } from "@/components/common";
import { ProjectRow } from "@/components/sidebar/ProjectRow";
import { SessionRow } from "@/components/sidebar/SessionRow";
import { useProjects, useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

// ---- brand ---------------------------------------------------------------

function FullBrand() {
  return (
    <>
      <div className="brand-mark"><Icon name="spark" size={16} /></div>
      <div>
        <div className="brand-name">Lyra</div>
      </div>
    </>
  );
}
function RailBrand() {
  return <div className="brand-mark"><Icon name="spark" size={16} /></div>;
}

export const sidebarBrand = definePlugin({
  name: "lyra.builtin.sidebar-brand",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.brand", { id: "brand", order: 0, component: FullBrand });
    host.layout.register("sidebar.rail.brand", { id: "rail-brand", order: 0, component: RailBrand });
  },
});

// ---- search --------------------------------------------------------------

// Currently a placeholder input — there's no global "search files /
// commands" implementation yet. A future plugin can replace this with a
// real local-files index.
function SidebarSearch() {
  return (
    <div className="side-search">
      <div className="side-search-icon"><Icon name="search" size={14} /></div>
      <input placeholder="Search · files · commands" />
      {/* search-kbd is absolutely positioned inside .side-search, so it
          needs that exact class — not the generic .kbd primitive. */}
      <span className="search-kbd">⌘K</span>
    </div>
  );
}

export const sidebarSearch = definePlugin({
  name: "lyra.builtin.sidebar-search",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.search", { id: "default", order: 0, component: SidebarSearch });
  },
});

// ---- Projects section ----------------------------------------------------

function ProjectsSection() {
  const { data: projects, isLoading } = useProjects();
  return (
    <>
      <SectionLabel
        trailing={<button className="add" title="Add project"><Icon name="plus" size={12} /></button>}
      >
        Projects
      </SectionLabel>
      {isLoading ? (
        <SkeletonList count={3} />
      ) : !projects || projects.length === 0 ? (
        <EmptyState
          icon="folder"
          title="No projects"
          sub="Add one to scope sessions to a codebase."
          size="compact"
        />
      ) : (
        <div className="side-list">
          {projects.map((p) => <ProjectRow key={p.id} project={p} />)}
        </div>
      )}
    </>
  );
}

export const sidebarProjects = definePlugin({
  name: "lyra.builtin.sidebar-projects",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerSection({ id: "projects", order: 0, component: ProjectsSection });
  },
});

// ---- Sessions section ----------------------------------------------------

function SessionsSection() {
  const { data: sessions, isLoading } = useSessions();
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const selectTab = useSessionStore((s) => s.selectTab);

  return (
    <>
      <SectionLabel trailing={<span className="count">{sessions?.length ?? 0}</span>}>
        Sessions
      </SectionLabel>
      {isLoading ? (
        <SkeletonList count={4} />
      ) : !sessions || sessions.length === 0 ? (
        <EmptyState
          icon="chat"
          title="No sessions yet"
          sub="Start a new conversation to see it here."
          size="compact"
        />
      ) : (
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
      )}
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

// ---- footer (user card) --------------------------------------------------

function SidebarFooter() {
  const openSettings = () =>
    useSessionStore.getState().openMainView({
      id: "settings",
      title: "Settings",
      icon: "settings",
    });

  return (
    <div className="user-card">
      <div className="user-avatar">J</div>
      <div className="user-body">
        <div className="user-name">Jamie Doe</div>
        <div className="user-sub">jdoe@longbridge-inc.com</div>
      </div>
      <button className="user-action" onClick={openSettings} title="Settings">
        <Icon name="settings" size={14} />
      </button>
      <button className="user-action" title="Account menu">
        <Icon name="more" size={14} />
      </button>
    </div>
  );
}

export const sidebarFooter = definePlugin({
  name: "lyra.builtin.sidebar-footer",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.footer", { id: "user-card", order: 0, component: SidebarFooter });
  },
});

// ---- rail (collapsed) actions -------------------------------------------

function NewSessionBtn() {
  return (
    <IconButton variant="rail-primary" title="New session">
      <Icon name="plus" size={16} />
    </IconButton>
  );
}
function SearchBtn() {
  return (
    <IconButton variant="rail" title="Search (⌘K)">
      <Icon name="search" size={16} />
    </IconButton>
  );
}

export const sidebarRailActions = definePlugin({
  name: "lyra.builtin.sidebar-rail-actions",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerRailItem({ id: "new-session", order: 10, component: NewSessionBtn });
    host.sidebar.registerRailItem({ id: "search",      order: 20, component: SearchBtn });
  },
});

// ---- rail (collapsed) sessions stack ------------------------------------

function RailSessions() {
  const { data: sessions = [] } = useSessions();
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const selectTab = useSessionStore((s) => s.selectTab);
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

export const sidebarRailSessions = definePlugin({
  name: "lyra.builtin.sidebar-rail-sessions",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerRailItem({ id: "rail-sessions", order: 100, component: RailSessions });
  },
});

// ---- rail (collapsed) bottom — tools / settings / user ------------------

function RailSpacer()  { return <div style={{ flex: 1 }} />; }
function RailTools()    {
  return <IconButton variant="rail" title="Tools / MCP"><Icon name="tool" size={16} /></IconButton>;
}
function RailSettings() {
  return <IconButton variant="rail" title="Settings"><Icon name="settings" size={16} /></IconButton>;
}
function RailUser()     {
  return <div className="rail-user" title="You · jdoe@longbridge-inc.com">J</div>;
}

export const sidebarRailBottom = definePlugin({
  name: "lyra.builtin.sidebar-rail-bottom",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerRailItem({ id: "rail-spacer",   order: 800, component: RailSpacer });
    host.sidebar.registerRailItem({ id: "rail-tools",    order: 900, component: RailTools });
    host.sidebar.registerRailItem({ id: "rail-settings", order: 910, component: RailSettings });
    host.sidebar.registerRailItem({ id: "rail-user",     order: 920, component: RailUser });
  },
});
