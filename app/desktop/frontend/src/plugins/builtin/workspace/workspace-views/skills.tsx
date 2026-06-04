// Built-in workspace view: "Skills" — the agent skills discovered in the
// session's cwd (workspace.listSkills). Read-only catalog; mirrors the
// Tools (MCP) view shape.

import { DataView } from "@/components/common";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useSkills } from "@/lib/data/queries";
import { defineWorkspaceView } from "./defineWorkspaceView";

function SkillsTab() {
  const { data, isLoading, isError } = useSkills();
  const skills = data ?? [];

  return (
    <WorkspaceViewLayout
      icon="sparkle"
      titleStrong
      title="Skills"
      sub={`${skills.length} available`}
      scrollClassName="py-1"
    >
      <DataView
        items={skills}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={4}
        empty={{
          icon: "sparkle",
          title: "No skills",
          sub: "Skills discovered in this project's working directory show up here.",
        }}
      >
        {(rows) => (
          <div className="flex flex-col">
            {rows.map((s) => (
              <div key={s.name} className="px-4 py-2">
                <div className="text-[13px] font-semibold text-fg">{s.name}</div>
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
  title: "Skills",
  icon: "sparkle",
  openByDefault: false,
  order: 45,
  component: SkillsTab,
});
