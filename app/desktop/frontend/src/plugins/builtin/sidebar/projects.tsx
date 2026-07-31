import { useState } from "react";
import { DataView, SectionLabel } from "@/ui";
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

// Project groups need enough separation to remain scannable; rows inside one
// project stay compact so the folder/session hierarchy reads as one unit.
const projectListClasses = "flex flex-col gap-3";
const sessionListClasses = "flex flex-col gap-0.5";

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
  onRename: (id: string, expectedRevision: number, title: string) => void;
  onFork: (id: string) => void;
  onDelete: (id: string) => void;
  onToggleFavorite: (id: string, expectedRevision: number, favorite: boolean) => void;
}) {
  const t = useT();
  const [open, setOpen] = useState(true);
  const [showAll, setShowAll] = useState(false);
  const visible = showAll ? group.sessions : group.sessions.slice(0, VISIBLE_CAP);
  const hidden = group.sessions.length - visible.length;

  return (
    <div className={sessionListClasses}>
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
        <div className="flex flex-col gap-0.5 pt-0.5">
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
              className="rounded-xs border-0 bg-transparent px-8 py-1 text-left text-ui-sm text-fg transition-colors hover:bg-hover"
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
      <SectionLabel className="flex h-7 items-center py-0">
        {t("workIndex.section.projects")}
      </SectionLabel>
      <DataView
        items={workIndex.groups}
        isLoading={workIndex.isLoading}
        isError={workIndex.isError}
        skeletonCount={3}
        skeletonVariant="compact"
        loadingLabel={t("common.loading")}
        empty={{
          title: t("projects.empty.title"),
          sub: t("projects.empty.sub"),
          size: "compact",
        }}
        error={{
          title: t("projects.error.title"),
          sub: t("projects.error.sub"),
          size: "compact",
        }}
      >
        {(items) => (
          <div className={projectListClasses}>
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
