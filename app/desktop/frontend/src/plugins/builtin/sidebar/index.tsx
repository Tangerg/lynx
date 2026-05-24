// Built-in plugins that fill the sidebar's slots — brand mark, search,
// projects/sessions sections, footer user-card, and the rail (collapsed)
// equivalents.
//
// Each piece is still its own plugin so a re-skinned build can swap any
// single contribution without forking the others. They live together
// because they're individually small.

import { DataView, Icon, IconButton, SectionLabel } from "@/components/common";
import { cn } from "@/lib/utils";
import { ProjectRow } from "@/components/sidebar/ProjectRow";
import { SessionRow } from "@/components/sidebar/SessionRow";
import { useProjects, useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

// ---- brand ---------------------------------------------------------------
//
// `.brand-mark` class is kept as a hook for the Wails drag-region opt-out
// defined in overlays.css (`.brand .brand-mark` / `.rail-brand .brand-mark`).
// All visual styling lives in Tailwind utilities right here.

function BrandMark({ extra }: { extra?: string }) {
  return (
    <div
      className={cn(
        // The dark accent (#1ed760) is luminous enough to read black ink;
        // the light accent (#15883e) is too dark — needs white.
        "brand-mark grid h-7 w-7 place-items-center rounded-full bg-accent text-black light:text-white",
        extra,
      )}
    >
      <Icon name="spark" size={16} />
    </div>
  );
}

function FullBrand() {
  return (
    <>
      <BrandMark />
      <div>
        <div className="font-sans text-[17px] font-extrabold tracking-[-0.01em]">Lyra</div>
      </div>
    </>
  );
}

// In rail mode the wrapper itself is the 36×36 accent square; brand-mark
// just hosts the icon (no extra background) and uses `contents` so it
// doesn't introduce a second visual box.
function RailBrand() {
  return (
    <div className="brand-mark contents">
      <Icon name="spark" size={16} />
    </div>
  );
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
//
// Currently a placeholder input — there's no global "search files /
// commands" implementation yet. A future plugin can replace this with a
// real local-files index.

function SidebarSearch() {
  return (
    <div className="relative mx-1 mb-3.5">
      <div className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-fg-faint">
        <Icon name="search" size={14} />
      </div>
      <input
        placeholder="Search · files · commands"
        // Dark mode: shadow-input/focus tokens give a soft inset glow.
        // Light mode: tokens go quiet (Vercel pattern) so we draw shape
        // with a hairline border + accent focus ring instead.
        className={cn(
          "w-full rounded-sm border-0 bg-surface-2 py-2 pl-9 pr-3 font-sans text-[13px] text-fg outline-none placeholder:text-fg-faint",
          "shadow-[var(--shadow-input)] focus:shadow-[var(--shadow-input-focus)]",
          "light:border light:border-line light:shadow-none",
          "light:focus:border-accent light:focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_12%,transparent)]",
        )}
      />
      <span className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded-[4px] bg-surface-2 light:bg-surface-3 px-1.5 py-px font-mono text-[11px] text-fg-faint tracking-normal">
        ⌘K
      </span>
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
        trailing={
          <button
            type="button"
            title="Add project"
            className="ml-auto grid h-6.5 w-6.5 place-items-center rounded-full border-0 bg-surface-2 text-fg-muted cursor-pointer transition-colors hover:bg-surface-3 hover:text-fg active:scale-[0.92]"
          >
            <Icon name="plus" size={12} />
          </button>
        }
      >
        Projects
      </SectionLabel>
      <DataView
        items={projects}
        isLoading={isLoading}
        skeletonCount={3}
        empty={{
          icon: "folder",
          title: "No projects",
          sub: "Add one to scope sessions to a codebase.",
          size: "compact",
        }}
      >
        {(items) => (
          <div className={SIDE_LIST}>
            {items.map((p) => <ProjectRow key={p.id} project={p} />)}
          </div>
        )}
      </DataView>
    </>
  );
}

const SIDE_LIST = "flex flex-col gap-0.5 px-1";

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
      <SectionLabel
        trailing={
          <span className="ml-auto rounded-full bg-surface-2 px-1.5 py-px text-[10px] text-fg-muted">
            {sessions?.length ?? 0}
          </span>
        }
      >
        Sessions
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
          <div className={SIDE_LIST}>
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

// ---- footer (user card) --------------------------------------------------

function SidebarFooter() {
  const openSettings = () =>
    useSessionStore.getState().openMainView({
      id: "settings",
      title: "Settings",
      icon: "settings",
    });

  return (
    <div className="grid grid-cols-[32px_minmax(0,1fr)_auto_auto] items-center gap-2.5 rounded-lg px-2.5 py-2 cursor-default transition-colors hover:bg-surface light:hover:bg-surface-2">
      {/* Avatar — 32px round, online-dot via `after:` pseudo-element. The
          dot's border matches the panel surface so it pops cleanly off
          both dark canvas and light surface. */}
      <div className="relative grid h-8 w-8 shrink-0 place-items-center rounded-full bg-surface-3 font-sans text-[13px] font-semibold text-fg after:content-[''] after:absolute after:-right-px after:-bottom-px after:h-2.5 after:w-2.5 after:rounded-full after:bg-accent after:border-2 after:border-canvas light:after:border-surface">
        J
      </div>
      <div className="min-w-0">
        <div className="truncate font-sans text-[13px] font-semibold leading-[1.2] text-fg">
          Jamie Doe
        </div>
        <div className="mt-px truncate text-[11px] text-fg-faint">
          jdoe@longbridge-inc.com
        </div>
      </div>
      <button type="button" onClick={openSettings} title="Settings" className={USER_ACTION}>
        <Icon name="settings" size={14} />
      </button>
      <button type="button" title="Account menu" className={USER_ACTION}>
        <Icon name="more" size={14} />
      </button>
    </div>
  );
}

const USER_ACTION =
  "grid h-6.5 w-6.5 place-items-center rounded-md border-0 bg-transparent text-fg-faint cursor-pointer transition-[background,color,transform] hover:bg-surface-2 hover:text-fg light:hover:bg-surface-3 active:scale-[0.92]";

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
      {/* hairline divider — kept as a tiny inline rule since the surface
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
              // Dark theme: surface (panel-bg) at rest, surface-2 on
              // hover / active. Light theme reads the same step
              // structure but starts one tone deeper because the
              // panel itself is white.
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

// ---- rail (collapsed) bottom — tools / settings / user ------------------

function RailSpacer()  { return <div className="flex-1" />; }
function RailTools()    {
  return <IconButton variant="rail" title="Tools / MCP"><Icon name="tool" size={16} /></IconButton>;
}
function RailSettings() {
  return <IconButton variant="rail" title="Settings"><Icon name="settings" size={16} /></IconButton>;
}
function RailUser()     {
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
    host.sidebar.registerRailItem({ id: "rail-spacer",   order: 800, component: RailSpacer });
    host.sidebar.registerRailItem({ id: "rail-tools",    order: 900, component: RailTools });
    host.sidebar.registerRailItem({ id: "rail-settings", order: 910, component: RailSettings });
    host.sidebar.registerRailItem({ id: "rail-user",     order: 920, component: RailUser });
  },
});
