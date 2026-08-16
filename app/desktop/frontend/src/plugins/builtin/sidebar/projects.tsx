import { useState } from "react";
import { DataView, SectionLabel } from "@/ui";
import { AgentWorkIndexGroupList } from "@/ui/agent";
import { ProjectRow } from "./ui/ProjectRow";
import { SessionList } from "./ui/SessionList";
import { useT } from "@/lib/i18n";
import type {
  WorkGroup,
  WorkIndexActions,
  WorkProject,
} from "@/plugins/builtin/navigation/public/workIndex";
import {
  contributeWorkIndexItem,
  useWorkIndex,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { definePlugin } from "@/plugins/sdk";

// One project node: header + (when open) its session list.
function ProjectGroupNode({
  group,
  actions,
  activeCwd,
  activeSessionId,
  onNewSession,
}: {
  group: WorkGroup;
  actions: WorkIndexActions;
  activeCwd: string | undefined;
  activeSessionId: string;
  onNewSession: (project: WorkProject) => void;
}) {
  const [open, setOpen] = useState(true);

  return (
    <div className="flex flex-col">
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
        <SessionList
          sessions={group.sessions}
          actions={actions}
          activeSessionId={activeSessionId}
          indented
          // The group is already ordered by recency, and the indent has taken
          // the width a timestamp would need out of the title.
          showTime={false}
        />
      )}
    </div>
  );
}

function ProjectsSection() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();

  return (
    <>
      <SectionLabel className="pt-0">{t("workIndex.section.projects")}</SectionLabel>
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
          <AgentWorkIndexGroupList>
            {items.map((group) => (
              <ProjectGroupNode
                key={group.project.id}
                group={group}
                actions={actions}
                activeCwd={workIndex.activeCwd}
                activeSessionId={workIndex.activeSessionId}
                onNewSession={(project) => actions.startSessionInFolder(project.id)}
              />
            ))}
          </AgentWorkIndexGroupList>
        )}
      </DataView>
    </>
  );
}

export const sidebarProjects = definePlugin({
  name: "lyra.builtin.sidebar-projects",
  setup(ctx) {
    contributeWorkIndexItem(ctx, {
      id: "projects",
      scope: "session",
      variant: "expanded",
      order: 0,
      component: ProjectsSection,
    });
  },
});
