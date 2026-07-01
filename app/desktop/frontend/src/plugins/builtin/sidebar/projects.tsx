// The sidebar's ONE workspace tree (Codex-style): projects are folder
// nodes, sessions nest under their project by cwd (project identity IS
// the cwd, AUX_API §1). Replaces the old flat Projects + Sessions pair —
// two lists describing one hierarchy was the un-converged version of
// this section.

import type { SidebarProject, SidebarSession } from "@/lib/data/queries";
import type { IconName } from "@/components/common";
import { useMemo, useState } from "react";
import { DataView, FIELD_CLASSES, Icon, SectionLabel } from "@/components/common";
import { ProjectRow } from "@/components/sidebar/ProjectRow";
import { SessionRow } from "@/components/sidebar/SessionRow";
import { useT } from "@/lib/i18n";
import { basename } from "@/lib/path";
import { useProjects, useSessions } from "@/lib/data/queries";
import {
  useActiveSessionCwd,
  useCreateSession,
  useDeleteSession,
  useForkSession,
  useRenameSession,
  useToggleFavorite,
} from "@/plugins/builtin/agent/public/session";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SIDEBAR_SECTION } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";
import { sideListClasses } from "./styles";

// Sessions shown per expanded project before the "Show more" fold —
// keeps a busy project from burying the ones below it (Codex's 展开显示).
const VISIBLE_CAP = 5;

const SESSION_DESTINATIONS: { id: string; icon: IconName; titleKey: string }[] = [
  { id: "explorer", icon: "folder", titleKey: "workspace.view.title.filetree" },
  { id: "files", icon: "filetext", titleKey: "workspace.view.title.files" },
  { id: "plan", icon: "list", titleKey: "workspace.view.title.plan" },
  { id: "todos", icon: "check", titleKey: "workspace.view.title.todos" },
];

interface ProjectGroup {
  project: SidebarProject;
  sessions: SidebarSession[];
}

// "+" — create a session in a chosen directory (sessions.create cwd). The
// runtime derives projects from session cwds, so "adding a project" IS
// creating the first session there; the inline input just asks for the path.
function AddProjectInline() {
  const t = useT();
  const createSession = useCreateSession();
  const [path, setPath] = useState("");

  const submit = (): void => {
    const cwd = path.trim();
    if (!cwd) return;
    setPath("");
    void createSession({ cwd });
  };

  return (
    <div className="px-3 pb-1.5">
      <div className="flex items-center gap-1.5 rounded-md border border-line bg-surface-2 px-2 py-1.5">
        <Icon name="plus" size={12} className="shrink-0 text-fg-faint" />
        <input
          type="text"
          value={path}
          onChange={(e) => setPath(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") submit();
          }}
          placeholder={t("sidebar.addProject.placeholder")}
          aria-label={t("sidebar.addProject.placeholder")}
          spellCheck={false}
          className={cn(FIELD_CLASSES, "h-5 flex-1 bg-transparent px-0 text-[12px] text-fg")}
        />
      </div>
    </div>
  );
}

// One project node: header + (when open) its capped session list.
function ProjectGroupNode({
  group,
  activeCwd,
  activeSessionId,
  activeMainView,
  forceExpand,
  onNewSession,
  onSelect,
  onRename,
  onFork,
  onDelete,
  onToggleFavorite,
}: {
  group: ProjectGroup;
  activeCwd: string | undefined;
  activeSessionId: string;
  activeMainView: string | null;
  /** While a filter is active: force the group open and show every match
   *  (ignore the collapse + VISIBLE_CAP fold), so results are never hidden. */
  forceExpand?: boolean;
  onNewSession: (project: SidebarProject) => void;
  onSelect: (id: string) => void;
  onRename: (id: string, title: string) => void;
  onFork: (id: string) => void;
  onDelete: (id: string) => void;
  onToggleFavorite: (id: string, favorite: boolean) => void;
}) {
  const t = useT();
  const [open, setOpen] = useState(true);
  const [showAll, setShowAll] = useState(false);
  const expanded = forceExpand || open;
  const visible = forceExpand || showAll ? group.sessions : group.sessions.slice(0, VISIBLE_CAP);
  const hidden = group.sessions.length - visible.length;

  return (
    <div className="flex flex-col gap-0.5">
      <ProjectRow
        project={group.project}
        // The accent bar marks the group only while it's collapsed — when
        // open, the nested session row carries the active state itself.
        active={group.project.id === activeCwd && !expanded}
        open={expanded}
        count={group.sessions.length}
        onToggle={() => setOpen((v) => !v)}
        onNewSession={onNewSession}
      />
      {expanded && group.sessions.length > 0 && (
        <div className="flex flex-col gap-0.5 pl-4">
          {visible.map((s) => (
            <div key={s.id} className="flex flex-col gap-0.5">
              <SessionRow
                session={s}
                active={s.id === activeSessionId}
                onSelect={onSelect}
                onRename={onRename}
                onFork={onFork}
                onDelete={onDelete}
                onToggleFavorite={onToggleFavorite}
              />
              {s.id === activeSessionId && (
                <div className="ml-5 flex flex-col gap-0.5 border-l border-divider pl-2">
                  {SESSION_DESTINATIONS.map((d) => {
                    const active = activeMainView === d.id;
                    return (
                      <button
                        key={d.id}
                        type="button"
                        data-chrome-focus=""
                        onClick={() => {
                          const store = useSessionStore.getState();
                          if (active) {
                            store.closeMainView(d.id);
                          } else {
                            store.openMainView({ id: d.id, title: d.titleKey, icon: d.icon });
                          }
                        }}
                        className={cn(
                          "flex items-center gap-2 rounded-md border-0 bg-transparent px-2 py-1.5 text-left text-[12.5px] text-fg-muted transition-colors duration-75",
                          "hover:bg-fg/[0.04] hover:text-fg focus-visible:bg-fg/[0.055] focus-visible:text-fg focus-visible:outline-none",
                          active && "bg-fg/[0.055] text-fg",
                        )}
                      >
                        <Icon name={d.icon} size={13} className="shrink-0 text-fg-faint" />
                        <span className="truncate">{t(d.titleKey)}</span>
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          ))}
          {(hidden > 0 || showAll) && (
            <button
              type="button"
              onClick={() => setShowAll((v) => !v)}
              className="rounded-lg border-0 bg-transparent px-2.5 py-1 text-left text-[11.5px] text-fg-faint transition-colors hover:bg-surface-2 hover:text-fg"
            >
              {hidden > 0 ? t("projects.showMore", { count: hidden }) : t("projects.showLess")}
            </button>
          )}
        </div>
      )}
    </div>
  );
}

function ProjectsSection() {
  const t = useT();
  const { data: projects, isLoading: projectsLoading, isError: projectsError } = useProjects();
  const { data: sessions, isLoading: sessionsLoading, isError: sessionsError } = useSessions();
  const draftIds = useSessionStore((s) => s.draftSessionIds);
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const activeMainView = useSessionStore((s) => s.activeMainView);
  const selectTab = useSessionStore((s) => s.selectTab);
  const createSession = useCreateSession();
  const deleteSession = useDeleteSession();
  const forkSession = useForkSession();
  const renameSession = useRenameSession();
  const toggleFavorite = useToggleFavorite();
  const activeCwd = useActiveSessionCwd();

  // Project nodes + their sessions (drafts hidden until first message, as
  // before). Sessions whose cwd has no project entry yet (cache timing /
  // serve-dir sessions) get a synthetic node from the cwd, so every
  // session stays reachable — the tree never silently drops one.
  const groups = useMemo<ProjectGroup[] | undefined>(() => {
    if (!projects && !sessions) return undefined;
    const byCwd = new Map<string, SidebarSession[]>();
    for (const s of sessions ?? []) {
      if (draftIds.has(s.id)) continue;
      const key = s.cwd ?? "";
      const list = byCwd.get(key);
      if (list) list.push(s);
      else byCwd.set(key, [s]);
    }
    // Favorited sessions pin to the top of their project group; the rest stay
    // newest-updated first.
    for (const list of byCwd.values()) {
      list.sort((a, b) => {
        if (Boolean(a.favorite) !== Boolean(b.favorite)) return a.favorite ? -1 : 1;
        return a.time < b.time ? 1 : -1;
      });
    }
    const result: ProjectGroup[] = (projects ?? []).map((p) => {
      const own = byCwd.get(p.id) ?? [];
      byCwd.delete(p.id);
      return { project: p, sessions: own };
    });
    for (const [cwd, list] of byCwd) {
      result.push({
        project: {
          id: cwd,
          name: cwd ? basename(cwd) : t("projects.fallbackName"),
          branch: "",
          sessionCount: list.length,
        },
        sessions: list,
      });
    }
    return result;
  }, [projects, sessions, draftIds, t]);

  const openProject = (project: SidebarProject): void => {
    void createSession({ cwd: project.id });
  };

  return (
    <>
      <SectionLabel>{t("sidebar.section.projects")}</SectionLabel>
      <AddProjectInline />
      <DataView
        items={groups}
        // Mirror the data we actually have, not the worst of the two queries:
        // once EITHER resolves `groups` is defined, so a partial failure (e.g.
        // projects errors but sessions loaded) still renders the available
        // sessions instead of blanking the list to a skeleton / error state.
        isLoading={(projectsLoading || sessionsLoading) && !groups}
        isError={(projectsError || sessionsError) && !groups}
        skeletonCount={3}
        empty={{
          icon: "folder",
          title: t("projects.empty.title"),
          sub: t("projects.empty.sub"),
          size: "compact",
        }}
      >
        {(items) => (
          <div className={sideListClasses}>
            {items.map((g) => (
              <ProjectGroupNode
                key={g.project.id}
                group={g}
                activeCwd={activeCwd}
                activeSessionId={activeSessionId}
                activeMainView={activeMainView}
                onNewSession={openProject}
                onSelect={selectTab}
                onRename={(id, title) => void renameSession(id, title)}
                onFork={forkSession}
                onDelete={deleteSession}
                onToggleFavorite={(id, fav) => void toggleFavorite(id, fav)}
              />
            ))}
          </div>
        )}
      </DataView>
    </>
  );
}

export const sidebarProjects = definePlugin({
  name: "lyra.builtin.sidebar-projects",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SIDEBAR_SECTION, {
      id: "projects",
      order: 0,
      component: ProjectsSection,
    });
  },
});
