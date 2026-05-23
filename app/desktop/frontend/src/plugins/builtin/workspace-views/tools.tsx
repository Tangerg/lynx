import { DataView, Icon, IconButton, ScrollArea } from "@/components/common";
import { ViewHeader } from "@/components/views/ViewHeader";
import { McpRow } from "@/components/views/McpRow";
import { useMCPServers } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";

const CONFIG_PATH = "~/.lyra/mcp.json";

function ToolsTab() {
  const { data, isLoading } = useMCPServers();
  const servers = data ?? [];
  const active = servers.filter((s) => s.status === "active").length;

  return (
    <>
      <ViewHeader
        icon="tool"
        titleStrong
        title="Connected MCP servers"
        sub={`${active} active · ${servers.length} configured`}
        actions={<IconButton title="Add server"><Icon name="plus" size={14} /></IconButton>}
      />
      <ScrollArea style={{ padding: "4px 0" }}>
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
              {rows.map((s) => <McpRow key={s.id} server={s} />)}
              <p className="view-foot">
                Servers expose tools the agent can call. Edit{" "}
                <code className="view-code">{CONFIG_PATH}</code> to add or remove.
              </p>
            </>
          )}
        </DataView>
      </ScrollArea>
    </>
  );
}

export const toolsView = definePlugin({
  name: "lyra.builtin.view-tools",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "tools",
      title: "Tools",
      icon: "tool",
      openByDefault: false,
      order: 40,
      component: ToolsTab,
    });
  },
});
