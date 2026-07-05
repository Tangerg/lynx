// Built-in workspace view: "Tools" — what the agent can call. Two
// catalogs with different lifecycles on one tab: the runtime's native
// tools (tools.list — static per runtime build) and the connected MCP
// servers (workspace.mcp.* — live 5-state lifecycle, expandable rows).

import { DataView, Icon } from "@/ui";
import { McpRow } from "./views/McpRow";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { defineWorkspaceView } from "./defineWorkspaceView";
import {
  builtinToolCatalogViewModel,
  toolCatalogSubtext,
  toolCatalogViewModel,
  useBuiltinToolConfigs,
  useMCPServerConfigs,
} from "@/plugins/builtin/workspace/application/toolCatalog";

function SectionHead({ children }: { children: React.ReactNode }) {
  return <div className="px-4 pt-2 pb-1 text-[10px] font-semibold text-fg-faint">{children}</div>;
}

function BuiltinToolsSection() {
  const t = useT();
  const { data, isLoading } = useBuiltinToolConfigs();
  const view = builtinToolCatalogViewModel(data ?? []);
  // No skeleton/error chrome here — the MCP DataView below owns the tab's
  // loading story; this section just appears once the catalog resolves.
  if (isLoading || view.isEmpty) return null;
  return (
    <div className="pb-1.5">
      <SectionHead>{t("tools.builtin")}</SectionHead>
      {view.rows.map((tool) => (
        <div
          key={tool.id}
          className="grid grid-cols-[auto_minmax(0,1fr)] items-baseline gap-2 px-4 py-1"
        >
          <code className="rounded-sm bg-surface-2 px-1 font-mono text-[11px] text-fg">
            {tool.name}
          </code>
          <div className="flex min-w-0 items-baseline gap-2">
            <span className="truncate text-[11.5px] text-fg-faint" title={tool.description}>
              {tool.description}
            </span>
            {tool.safety && (
              <span
                className={`shrink-0 rounded-sm px-1.5 py-0.5 font-mono text-[10px] ${tool.safety.className}`}
              >
                {tool.safety.label}
              </span>
            )}
          </div>
        </div>
      ))}
      <SectionHead>{t("tools.mcp")}</SectionHead>
    </div>
  );
}

function openMcpSettings(title: string): void {
  openWorkspaceSettingsPane("mcp-servers", title);
}

function ToolsTab() {
  const t = useT();
  const { data, isLoading, isError } = useMCPServerConfigs();
  const view = toolCatalogViewModel(data ?? []);

  return (
    <WorkspaceViewLayout
      icon="tool"
      titleStrong
      title="tools.title"
      sub={toolCatalogSubtext(view)}
      scrollClassName="py-1"
    >
      <BuiltinToolsSection />
      <DataView
        items={view.mcpServers}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={4}
        empty={{
          icon: "tool",
          title: t("tools.empty.title"),
          sub: t("tools.empty.sub"),
        }}
      >
        {(rows) => (
          <>
            {rows.map((s) => (
              <McpRow key={s.id} server={s} />
            ))}
            <button
              type="button"
              onClick={() => openMcpSettings(t("settings.title"))}
              className="m-0 flex items-center gap-1.5 px-4 pt-3.5 pb-4.5 text-[11px] leading-[1.5] text-fg-muted hover:text-fg"
            >
              <Icon name="settings" size={12} />
              {t("tools.footer")}
            </button>
          </>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const toolsView = defineWorkspaceView({
  id: "tools",
  title: "workspace.view.title.tools",
  icon: "tool",
  order: 40,
  splittable: true,
  component: ToolsTab,
});
