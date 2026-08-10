// Built-in workspace view: "Tools" — what the agent can call. Two
// catalogs with different lifecycles on one tab: the runtime's native
// tools (tools.list — static per runtime build) and the connected MCP
// servers (mcp.* — live 5-state lifecycle, expandable rows).

import { MCP_SERVERS_PANE } from "@/plugins/builtin/settings/public/panes";
import type { IconName } from "@/ui";
import { Badge, DataView, Icon, SectionLabel, TextButton } from "@/ui";
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

function SectionHead({ children, count }: { children: React.ReactNode; count?: number }) {
  return (
    // The count goes in the atom's own trailing slot. A family is worth scanning
    // past or stopping at, and how many calls it holds is the fact that decides
    // which — the same reason a tool group's row carries its count.
    <SectionLabel
      className="px-4 pb-1"
      trailing={count === undefined ? undefined : <span className="font-mono">{count}</span>}
    >
      {children}
    </SectionLabel>
  );
}

/**
 * What the agent can call, in families.
 *
 * A flat alphabetical list of thirty names answers "is X here"; the families answer
 * "what can it do", which is what someone opens this for. Each row leads with the
 * glyph that same tool wears in the transcript, so a card scrolled past and an
 * entry read here are recognisably the same thing.
 */
function BuiltinToolsSection() {
  const t = useT();
  const { data, isLoading } = useBuiltinToolConfigs();
  const view = builtinToolCatalogViewModel(data ?? []);
  // No skeleton/error chrome here — the MCP DataView below owns the tab's
  // loading story; this section just appears once the catalog resolves.
  if (isLoading || view.isEmpty) return null;
  return (
    <div className="pb-1.5">
      {view.families.map((family) => (
        <div key={family.id} className="pb-1">
          <SectionHead count={family.rows.length}>{t(family.titleKey)}</SectionHead>
          {family.rows.map((tool) => (
            <div
              key={tool.id}
              className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-2.5 px-4 py-1"
            >
              <Icon name={tool.icon as IconName} size="xs" className="mt-1 text-fg-faint" />
              <div className="min-w-0">
                <div className="flex min-w-0 items-baseline gap-2">
                  {/* Plain mono, no chip fill: with a glyph beside it and a heading
                      above, a filled name was the third mark competing on one row —
                      and the one chip a row still wears has to be the safety class,
                      which is the fact that changes what a call can do to you. */}
                  <span className="truncate font-mono text-ui-sm text-fg">{tool.name}</span>
                  {tool.safety && (
                    <Badge tone={tool.safety.tone} className="font-mono">
                      {tool.safety.label}
                    </Badge>
                  )}
                </div>
                {/* Its own line rather than sharing the name's: in a dock this narrow
                    both ended up truncated, so neither could be read. */}
                <div className="truncate text-ui-xs text-fg-faint" title={tool.description}>
                  {tool.description}
                </div>
              </div>
            </div>
          ))}
        </div>
      ))}
      <SectionHead>{t("tools.mcp")}</SectionHead>
    </div>
  );
}

function openMcpSettings(): void {
  openWorkspaceSettingsPane(MCP_SERVERS_PANE);
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
      sub={toolCatalogSubtext(t, view)}
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
        {(rows) => rows.map((s) => <McpRow key={s.id} server={s} />)}
      </DataView>
      {/* Outside the DataView, so it is there in every state. It used to live in the
          non-empty branch, which withheld the way to connect a server from exactly
          the install that has none — and left this pane with no focusable content at
          all, which is a scroll region a keyboard cannot reach. */}
      <TextButton size="sm" onClick={openMcpSettings} className="px-4 pt-3.5 pb-4.5 leading-body">
        <Icon name="settings" size="xs" />
        {t("tools.footer")}
      </TextButton>
    </WorkspaceViewLayout>
  );
}

export const toolsView = defineWorkspaceView({
  id: "tools",
  title: "workspace.view.title.tools",
  icon: "tool",
  order: 70,
  splittable: true,
  component: ToolsTab,
});
