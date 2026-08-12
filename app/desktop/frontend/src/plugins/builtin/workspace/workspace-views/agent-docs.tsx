// Built-in workspace view: "Agent docs" — the AGENTS.md files discovered
// from the session's cwd upward (agentDocs.list). Read-only.

import { DataView } from "@/ui";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useT } from "@/lib/i18n";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useWorkspaceAgentDocs } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { workspaceAgentDocsViewModel } from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";

function AgentDocsTab() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError } = useWorkspaceAgentDocs(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
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
        isLoading={isLoading || workspace.status === "resolving"}
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
                  <div className="truncate text-ui-md font-semibold text-fg">{d.title}</div>
                  <div className="mt-0.5 truncate font-mono text-ui-sm text-fg-faint">{d.path}</div>
                </div>
                <span className="rounded-full bg-surface-2 px-1.5 py-px text-ui-xs text-fg-muted">
                  {t(d.scopeLabelKey)}
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
  order: 110,
  splittable: true,
  component: AgentDocsTab,
});
