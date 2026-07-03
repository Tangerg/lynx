// Built-in workspace view: "Skills" — the agent skills discovered in the
// session's cwd (workspace.listSkills). Read-only catalog; mirrors the
// Tools (MCP) view shape.

import { DataView } from "@/components/common";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useWorkspaceSkills } from "@/plugins/builtin/workspace/application/workspaceData";
import { useWorkspaceCapability } from "@/plugins/builtin/workspace/application/workspaceCapabilities";

function SkillsTab() {
  const t = useT();
  const skillsEnabled = useWorkspaceCapability("skills");
  const { data, isLoading, isError } = useWorkspaceSkills();
  const skills = data ?? [];

  return (
    <WorkspaceViewLayout
      icon="sparkle"
      titleStrong
      title="skills.title"
      sub={skillsEnabled ? t("skills.available", { count: skills.length }) : t("skills.off")}
      scrollClassName="py-1"
    >
      <DataView
        items={skills}
        isLoading={isLoading}
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
              <div key={s.name} className="px-4 py-2">
                <div className="flex items-center gap-2">
                  <div className="text-[13px] font-semibold text-fg truncate">{s.name}</div>
                  {/* Layer the skill came from — "project" (<cwd>/.lyra/skills)
                      overrides "global" (user dir) on name collision, matching
                      the priority the runtime feeds the model. */}
                  {s.source && (
                    <span className="rounded-xs bg-surface-2 px-1.5 py-px font-mono text-[10px] text-fg-faint">
                      {s.source}
                    </span>
                  )}
                </div>
                {s.description && (
                  <div className="mt-0.5 text-[11.5px] leading-[1.45] text-fg-muted">
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
  order: 45,
  splittable: true,
  component: SkillsTab,
});
