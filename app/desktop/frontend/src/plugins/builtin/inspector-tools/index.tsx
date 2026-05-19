import { Icon, IconButton, ScrollArea } from "@/components/common";
import { McpRow } from "@/components/inspector/McpRow";
import { useMCPServers } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";

function ToolsTab() {
  const { data } = useMCPServers();
  const servers = data ?? [];
  const active = servers.filter((s) => s.status === "active").length;
  const configPath = "~/.lyra/mcp.json";

  return (
    <>
      <div className="insp-head">
        <div className="ficon"><Icon name="tool" size={14} /></div>
        <div style={{ minWidth: 0 }}>
          <div className="ftitle" style={{ fontFamily: "var(--font-ui)", fontSize: 13, fontWeight: 700 }}>
            Connected MCP servers
          </div>
          <div className="fsub">{active} active · {servers.length} configured</div>
        </div>
        <div style={{ display: "flex", gap: 4 }}>
          <IconButton title="Add server"><Icon name="plus" size={14} /></IconButton>
        </div>
      </div>
      <ScrollArea style={{ padding: "4px 0" }}>
        {servers.map((s) => <McpRow key={s.id} server={s} />)}
        <div style={{ padding: "14px 16px 18px 16px", color: "var(--color-text-faint)", fontSize: 11, lineHeight: 1.5 }}>
          Servers expose tools the agent can call. Edit{" "}
          <code style={{
            fontFamily: "var(--font-mono)", background: "var(--color-surface-2)",
            padding: "1px 5px", borderRadius: 3, color: "var(--color-text)",
          }}>
            {configPath}
          </code>{" "}
          to add or remove.
        </div>
      </ScrollArea>
    </>
  );
}

function useToolsBadge(): number | undefined {
  const { data } = useMCPServers();
  return data?.filter((s) => s.status === "active").length;
}

export default definePlugin({
  name: "lyra.builtin.inspector-tools",
  version: "1.0.0",
  setup({ host }) {
    host.inspector.registerTab({
      id: "tools",
      label: "Tools",
      icon: "tool",
      order: 40,
      useBadge: useToolsBadge,
      component: ToolsTab,
    });
  },
});
