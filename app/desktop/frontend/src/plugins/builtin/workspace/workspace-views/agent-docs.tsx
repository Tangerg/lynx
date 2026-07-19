// Built-in workspace view: "Agent docs" — the AGENTS.md files discovered
// from the session's cwd upward (agentDocs.list). Read-only.

import { DataView } from "@/ui";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useT } from "@/lib/i18n";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useWorkspaceAgentDocs } from "@/plugins/builtin/workspace/application/workspaceData";
import { workspaceAgentDocsViewModel } from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";

function AgentDocsTab() {
  const t = useT();
  const { data, isLoading, isError } = useWorkspaceAgentDocs();
  const view = workspaceAgentDocsViewModel(data ?? []);

  return (
    <WorkspaceViewLayout
      icon="book"
      titleStrong
      title="agentDocs.title"
      sub={t("agentDocs.found", { count: view.count })}
      scrollClassName="py-1"
    >
      <DataView
        items={view.rows}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "book",
          title: t("agentDocs.empty.title"),
          sub: t("agentDocs.empty.sub"),
        }}
      >
        {(rows) => (
          <div className="flex flex-col">
            {rows.map((d) => (
              <div
                key={d.id}
                className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-2 px-4 py-2"
              >
                <div className="min-w-0">
                  <div className="truncate text-[13px] font-semibold text-fg">{d.title}</div>
                  <div className="mt-0.5 truncate font-mono text-[11px] text-fg-faint">
                    {d.path}
                  </div>
                </div>
                <span className="rounded-full bg-surface-2 px-1.5 py-px text-[10px] text-fg-muted">
                  {d.scopeLabel}
                </span>
              </div>
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const agentDocsView = defineWorkspaceView({
  id: "agent-docs",
  title: "workspace.view.title.agentDocs",
  icon: "book",
  order: 46,
  component: AgentDocsTab,
});
