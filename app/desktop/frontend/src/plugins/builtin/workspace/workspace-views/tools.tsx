import { DataView, Icon, IconButton, ScrollArea } from "@/components/common";
import { McpRow } from "./views/McpRow";
import { ViewHeader } from "./views/ViewHeader";
import { useMCPServers } from "@/lib/data/queries";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";

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
        actions={
          <IconButton title="Add server">
            <Icon name="plus" size={14} />
          </IconButton>
        }
      />
      <ScrollArea className="py-1">
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
      </ScrollArea>
    </>
  );
}

export const toolsView = definePlugin({
  name: "lyra.builtin.view-tools",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(WORKSPACE_VIEW, {
      id: "tools",
      title: "Tools",
      icon: "tool",
      openByDefault: false,
      order: 40,
      component: ToolsTab,
    });
  },
});
