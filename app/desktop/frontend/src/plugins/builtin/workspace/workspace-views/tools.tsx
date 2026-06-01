import { DataView, Icon, IconButton } from "@/components/common";
import { McpRow } from "./views/McpRow";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useMCPServers } from "@/lib/data/queries";
import { defineWorkspaceView } from "./defineWorkspaceView";

const CONFIG_PATH = "~/.lyra/mcp.json";

function ToolsTab() {
  const { data, isLoading } = useMCPServers();
  const servers = data ?? [];
  const active = servers.filter((s) => s.status === "active").length;

  return (
    <WorkspaceViewLayout
      icon="tool"
      titleStrong
      title="Connected MCP servers"
      sub={`${active} active · ${servers.length} configured`}
      scrollClassName="py-1"
      actions={
        <IconButton title="Add server">
          <Icon name="plus" size={14} />
        </IconButton>
      }
    >
      <DataView
        items={servers}
        isLoading={isLoading}
        skeletonCount={4}
        empty={{
          icon: "tool",
          title: "No MCP servers configured",
          sub: `Add a server in ${CONFIG_PATH} to expose tools to the agent.`,
        }}
      >
        {(rows) => (
          <>
            {rows.map((s) => (
              <McpRow key={s.id} server={s} />
            ))}
            <p className="m-0 px-4 pt-3.5 pb-4.5 text-[11px] leading-[1.5] text-fg-faint">
              Servers expose tools the agent can call. Edit{" "}
              <code className="rounded-xs bg-surface-2 px-1.5 py-px font-mono text-fg">
                {CONFIG_PATH}
              </code>{" "}
              to add or remove.
            </p>
          </>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const toolsView = defineWorkspaceView({
  id: "tools",
  title: "Tools",
  icon: "tool",
  openByDefault: false,
  order: 40,
  component: ToolsTab,
});
