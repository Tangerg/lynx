import { useState } from "react";
import { AgentSectionLabel } from "@/ui/agent";
import { DataView } from "@/ui";
import { ProjectRow } from "./ui/ProjectRow";
import { SessionRow } from "./ui/SessionRow";
import { useT } from "@/lib/i18n";
import type { WorkGroup, WorkProject } from "@/plugins/builtin/navigation/public/workIndex";
import {
  contributeWorkIndexItem,
  useWorkIndex,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { definePlugin } from "@/plugins/sdk";

// Sessions shown per expanded project before the "Show more" fold —
// keeps a busy project from burying the ones below it (Codex's 展开显示).
const VISIBLE_CAP = 5;

// Vertical list column — the section list and each project's nested session list.
const sideListClasses = "flex flex-col gap-0.5";

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
  onToggleFavorite,
}: {
  group: WorkGroup;
  activeCwd: string | undefined;
  activeSessionId: string;
  onNewSession: (project: WorkProject) => void;
  onSelect: (id: string) => void;
  onRename: (id: string, title: string) => void;
  onFork: (id: string) => void;
  onDelete: (id: string) => void;
  onToggleFavorite: (id: string, favorite: boolean) => void;
}) {
  const t = useT();
  const [open, setOpen] = useState(true);
  const [showAll, setShowAll] = useState(false);
  const visible = showAll ? group.sessions : group.sessions.slice(0, VISIBLE_CAP);
  const hidden = group.sessions.length - visible.length;

  return (
    <div className={sideListClasses}>
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
        <div className={sideListClasses}>
          {visible.map((s) => (
            <SessionRow
              key={s.id}
              session={s}
              active={s.id === activeSessionId}
              onSelect={onSelect}
              onRename={onRename}
              onFork={onFork}
              onDelete={onDelete}
              onToggleFavorite={onToggleFavorite}
            />
          ))}
          {(hidden > 0 || showAll) && (
            <button
              type="button"
              onClick={() => setShowAll((v) => !v)}
              className="rounded-[7px] border-0 bg-transparent px-7 py-1 text-left text-[11.5px] text-fg-faint transition-colors hover:bg-fg/[0.045] hover:text-fg"
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
  const workIndex = useWorkIndex({ fallbackProjectName: t("projects.fallbackName") });
  const actions = useWorkIndexActions();

  const startSessionInFolder = (project: WorkProject): void => {
    actions.startSessionInFolder(project.id);
  };

  return (
    <>
      <AgentSectionLabel>{t("workIndex.section.projects")}</AgentSectionLabel>
      <DataView
        items={workIndex.groups}
        isLoading={workIndex.isLoading}
        isError={workIndex.isError}
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
                activeCwd={workIndex.activeCwd}
                activeSessionId={workIndex.activeSessionId}
                onNewSession={startSessionInFolder}
                onSelect={actions.selectSession}
                onRename={actions.renameSession}
                onFork={actions.forkSession}
                onDelete={actions.deleteSession}
                onToggleFavorite={actions.toggleFavorite}
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
    contributeWorkIndexItem(host, {
      id: "projects",
      scope: "session",
      variant: "expanded",
      order: 0,
      component: ProjectsSection,
    });
  },
});
