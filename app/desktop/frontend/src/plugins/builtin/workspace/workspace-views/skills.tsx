// Built-in workspace view: "Skills" — the agent skills discovered in the
// session's cwd (skills.discovered.list). Read-only catalog; mirrors the
// Tools (MCP) view shape.

import { DataView } from "@/ui";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useWorkspaceSkills } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { useWorkspaceCapability } from "@/plugins/builtin/workspace/application/workspaceCapabilities";
import { workspaceSkillsViewModel } from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";

function SkillsTab() {
  const t = useT();
  const skillsEnabled = useWorkspaceCapability("skills");
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError } = useWorkspaceSkills(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
  const view = workspaceSkillsViewModel(data ?? [], skillsEnabled);

  return (
    <WorkspaceViewLayout
      icon="sparkle"
      titleStrong
      title="skills.title"
      sub={view.enabled ? t("skills.available", { count: view.count }) : t("skills.off")}
      scrollClassName="py-1"
    >
      <DataView
        items={view.rows}
        isLoading={view.enabled && (isLoading || workspace.status === "resolving")}
        isError={isError}
        skeletonCount={4}
        empty={
          skillsEnabled
            ? {
                icon: "sparkle",
                title: t("skills.empty.title"),
                sub: t("skills.empty.sub"),
              }
            : {
                icon: "sparkle",
                title: t("skills.disabled.title"),
                sub: t("skills.disabled.sub"),
              }
        }
      >
        {(rows) => (
          <div className="flex flex-col">
            {rows.map((s) => (
              <div key={s.id} className="px-4 py-2">
                <div className="flex items-center gap-2">
                  <div className="text-ui-md font-semibold text-fg truncate">{s.name}</div>
                  {s.scope && (
                    <span className="rounded-sm bg-surface-2 px-1.5 py-px font-mono text-ui-xs text-fg-faint">
                      {s.scope}
                    </span>
                  )}
                </div>
                {s.description && (
                  <div className="mt-0.5 text-ui-sm leading-body text-fg-muted">
                    {s.description}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const skillsView = defineWorkspaceView({
  id: "skills",
  title: "workspace.view.title.skills",
  icon: "sparkle",
  order: 80,
  splittable: true,
  component: SkillsTab,
});
