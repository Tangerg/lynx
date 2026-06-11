// Built-in workspace view: "Tools" — what the agent can call. Two
// catalogs with different lifecycles on one tab: the runtime's native
// tools (tools.list — static per runtime build) and the connected MCP
// servers (workspace.mcp.* — live 5-state lifecycle, expandable rows).

import { DataView } from "@/components/common";
import { McpRow } from "./views/McpRow";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useBuiltinTools, useMCPServers } from "@/lib/data/queries";
import { defineWorkspaceView } from "./defineWorkspaceView";

const CONFIG_PATH = "~/.lyra/mcp.json";

// Safety class → pill tint. Unknown classes fall back to the neutral pill
// (forward-compat: the enum is open on the wire).
const SAFETY_PILL: Record<string, string> = {
  safe: "bg-accent/12 text-accent",
  write: "bg-warning/12 text-warning",
  exec: "bg-negative/12 text-negative",
  network: "bg-surface-2 text-fg-muted",
};

function SectionHead({ children }: { children: string }) {
  return (
    <div className="px-4 pt-2 pb-1 text-[10px] font-semibold tracking-wider text-fg-faint uppercase">
      {children}
    </div>
  );
}

function BuiltinToolsSection() {
  const { data, isLoading } = useBuiltinTools();
  // No skeleton/error chrome here — the MCP DataView below owns the tab's
  // loading story; this section just appears once the catalog resolves.
  if (isLoading || !data?.length) return null;
  return (
    <div className="pb-1.5">
      <SectionHead>Built-in tools</SectionHead>
      {data.map((tool) => (
        <div
          key={tool.name}
          className="grid grid-cols-[auto_minmax(0,1fr)] items-baseline gap-2 px-4 py-1"
        >
          <code className="rounded-xs bg-surface-2 px-1 font-mono text-[11px] text-fg">
            {tool.name}
          </code>
          <div className="flex min-w-0 items-baseline gap-2">
            <span className="truncate text-[11.5px] text-fg-faint">{tool.description}</span>
            {tool.safetyClass && (
              <span
                className={`shrink-0 rounded-xs px-1.5 py-0.5 font-mono text-[10px] ${SAFETY_PILL[tool.safetyClass] ?? "bg-surface-2 text-fg-muted"}`}
              >
                {tool.safetyClass}
              </span>
            )}
          </div>
        </div>
      ))}
      <SectionHead>MCP servers</SectionHead>
    </div>
  );
}

function ToolsTab() {
  const { data, isLoading, isError } = useMCPServers();
  const servers = data ?? [];
  const active = servers.filter((s) => s.status === "connected").length;

  return (
    <WorkspaceViewLayout
      icon="tool"
      titleStrong
      title="Tools"
      sub={`${active} MCP active · ${servers.length} configured`}
      scrollClassName="py-1"
    >
      <BuiltinToolsSection />
      <DataView
        items={servers}
        isLoading={isLoading}
        isError={isError}
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
