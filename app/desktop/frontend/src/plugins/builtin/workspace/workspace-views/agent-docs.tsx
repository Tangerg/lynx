// Built-in workspace view: "Agent docs" — the AGENTS.md files discovered
// from the session's cwd upward (workspace.listAgentDocs). Read-only.

import { DataView } from "@/components/common";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useAgentDocs } from "@/lib/data/queries";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { scopeLabel } from "./views/scopeLabel";

function AgentDocsTab() {
  const { data, isLoading, isError } = useAgentDocs();
  const docs = data ?? [];

  return (
    <WorkspaceViewLayout
      icon="book"
      titleStrong
      title="Agent docs"
      sub={`${docs.length} found`}
      scrollClassName="py-1"
    >
      <DataView
        items={docs}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "book",
          title: "No agent docs",
          sub: "AGENTS.md files found from the working directory upward show up here.",
        }}
      >
        {(rows) => (
          <div className="flex flex-col">
            {rows.map((d) => (
              <div
                key={d.path}
                className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-2 px-4 py-2"
              >
                <div className="min-w-0">
                  <div className="truncate text-[13px] font-semibold text-fg">
                    {d.title || d.path}
                  </div>
                  <div className="mt-0.5 truncate font-mono text-[11px] text-fg-faint">
                    {d.path}
                  </div>
                </div>
                <span className="rounded-full bg-surface-2 px-1.5 py-px text-[10px] text-fg-muted">
                  {scopeLabel(d.scope)}
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
  openByDefault: false,
  order: 46,
  component: AgentDocsTab,
});
