// The sidebar's ONE workspace tree (Codex-style): projects are folder
// nodes, sessions nest under their project by cwd (project identity IS
// the cwd, AUX_API §1). Replaces the old flat Projects + Sessions pair —
// two lists describing one hierarchy was the un-converged version of
// this section.

import type { SidebarProject, SidebarSession } from "@/lib/data/queries";
import { useMemo, useState } from "react";
import * as Popover from "@radix-ui/react-popover";
import { DataView, FIELD_CLASSES, Icon, SectionLabel } from "@/components/common";
import { ProjectRow } from "@/components/sidebar/ProjectRow";
import { SessionRow } from "@/components/sidebar/SessionRow";
import { useT } from "@/lib/i18n";
import { useProjects, useSessions } from "@/lib/data/queries";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { useCreateSession } from "@/lib/agent/useCreateSession";
import { useDeleteSession } from "@/lib/agent/useDeleteSession";
import { useForkSession } from "@/lib/agent/useForkSession";
import { useRenameSession } from "@/lib/agent/useRenameSession";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SIDEBAR_SECTION } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";
import { sideListClasses } from "./styles";

// Sessions shown per expanded project before the "Show more" fold —
// keeps a busy project from burying the ones below it (Codex's 展开显示).
const VISIBLE_CAP = 5;

interface ProjectGroup {
  project: SidebarProject;
  sessions: SidebarSession[];
}

// "+" — create a session in a chosen directory (sessions.create cwd). The
// runtime derives projects from session cwds, so "adding a project" IS
// creating the first session there; the popover just asks for the path.
function AddProjectButton() {
  const t = useT();
  const createSession = useCreateSession();
  const [open, setOpen] = useState(false);
  const [path, setPath] = useState("");

  const submit = (): void => {
    const cwd = path.trim();
    if (!cwd) return;
    setOpen(false);
    setPath("");
    void createSession({ cwd });
  };

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button
          type="button"
          title={t("sidebar.action.addProject")}
          aria-label={t("sidebar.action.addProject")}
          className="ml-auto grid h-6.5 w-6.5 place-items-center rounded-full border-0 bg-surface-2 text-fg-muted transition-colors hover:bg-surface-3 hover:text-fg active:scale-[0.92]"
        >
          <Icon name="plus" size={12} />
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side="bottom"
          align="start"
          sideOffset={6}
          className="z-50 w-[300px] rounded-lg border border-line bg-surface p-3 shadow-lg"
        >
          <div className="mb-2 text-[11px] font-semibold tracking-wider text-fg-faint uppercase">
            {t("sidebar.addProject.title")}
          </div>
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
            className={cn(FIELD_CLASSES, "h-7 w-full px-2 text-fg")}
          />
          <div className="mt-1.5 text-[10.5px] leading-[1.4] text-fg-faint">
            {t("sidebar.addProject.hint")}
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

// One project node: header + (when open) its capped session list.
function ProjectGroupNode({
  group,
  activeCwd,
  activeSessionId,
  onNewSession,
  onSelect,
  onRename,
  onFork,
  onDelete,
}: {
  group: ProjectGroup;
  activeCwd: string | undefined;
  activeSessionId: string;
  onNewSession: (project: SidebarProject) => void;
  onSelect: (id: string) => void;
  onRename: (id: string, title: string) => void;
  onFork: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  const [open, setOpen] = useState(true);
  const [showAll, setShowAll] = useState(false);
  const visible = showAll ? group.sessions : group.sessions.slice(0, VISIBLE_CAP);
  const hidden = group.sessions.length - visible.length;

  return (
    <div className="flex flex-col gap-0.5">
      <ProjectRow
        project={group.project}
        // The accent bar marks the group only while it's collapsed — when
        // open, the nested session row carries the active state itself.
        active={group.project.id === activeCwd && !open}
        open={open}
        count={group.sessions.length}
        onToggle={() => setOpen((v) => !v)}
        onNewSession={onNewSession}
      />
      {open && group.sessions.length > 0 && (
        <div className="flex flex-col gap-0.5 pl-4">
          {visible.map((s) => (
            <SessionRow
              key={s.id}
              session={s}
              active={s.id === activeSessionId}
              onSelect={onSelect}
              onRename={onRename}
              onFork={onFork}
              onDelete={onDelete}
            />
          ))}
          {(hidden > 0 || showAll) && (
            <button
              type="button"
              onClick={() => setShowAll((v) => !v)}
              className="rounded-lg border-0 bg-transparent px-2.5 py-1 text-left text-[11.5px] text-fg-faint transition-colors hover:bg-surface-2 hover:text-fg"
            >
              {hidden > 0 ? `Show ${hidden} more` : "Show less"}
            </button>
          )}
        </div>
      )}
    </div>
  );
}

function basename(cwd: string): string {
  return cwd.replace(/\/+$/, "").split("/").at(-1) || cwd;
}

function ProjectsSection() {
  const t = useT();
  const { data: projects, isLoading: projectsLoading, isError: projectsError } = useProjects();
  const { data: sessions, isLoading: sessionsLoading, isError: sessionsError } = useSessions();
  const draftIds = useSessionStore((s) => s.draftSessionIds);
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const selectTab = useSessionStore((s) => s.selectTab);
  const createSession = useCreateSession();
  const deleteSession = useDeleteSession();
  const forkSession = useForkSession();
  const renameSession = useRenameSession();
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
    for (const list of byCwd.values()) list.sort((a, b) => (a.time < b.time ? 1 : -1));
    const result: ProjectGroup[] = (projects ?? []).map((p) => {
      const own = byCwd.get(p.id) ?? [];
      byCwd.delete(p.id);
      return { project: p, sessions: own };
    });
    for (const [cwd, list] of byCwd) {
      result.push({
        project: {
          id: cwd,
          name: cwd ? basename(cwd) : "Other",
          branch: "",
          sessionCount: list.length,
        },
        sessions: list,
      });
    }
    return result;
  }, [projects, sessions, draftIds]);

  const openProject = (project: SidebarProject): void => {
    void createSession({ cwd: project.id });
  };

  return (
    <>
      <SectionLabel trailing={<AddProjectButton />}>{t("sidebar.section.projects")}</SectionLabel>
      <DataView
        items={groups}
        isLoading={projectsLoading || sessionsLoading}
        isError={projectsError && sessionsError}
        skeletonCount={3}
        empty={{
          icon: "folder",
          title: "No projects",
          sub: "Add one to scope sessions to a codebase.",
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
                onNewSession={openProject}
                onSelect={selectTab}
                onRename={(id, title) => void renameSession(id, title)}
                onFork={forkSession}
                onDelete={deleteSession}
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
