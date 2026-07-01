// Per-tool gating for one connected MCP server. For each tool the server
// advertises (workspace.mcp.listTools) it renders two switches:
//
//   - Enabled    — off ⇒ the tool joins disabledTools, hidden from the model.
//   - Auto-approve — on ⇒ the tool joins autoApproveTools, skipping the approval
//                    prompt. Disabled (and forced off) while the tool itself is
//                    disabled — a hidden tool can never be called, so pre-
//                    approving it is meaningless.
//
// The parent (ServerForm) owns the two lists and persists them on Save; this
// component is the controlled editor over them. Both lists key on the BARE tool
// name (the server scopes them); the runtime qualifies to "<server>_<tool>"
// when it enforces. The lists are sparse by design — only non-default entries
// are stored (every tool enabled, none auto-approved unless listed).

import { DataView, Switch } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useMCPServerTools } from "./application/mcpServerTools";

interface Props {
  server: string;
  disabledTools: string[];
  autoApproveTools: string[];
  onChange: (next: { disabledTools: string[]; autoApproveTools: string[] }) => void;
}

export function ToolControls({ server, disabledTools, autoApproveTools, onChange }: Props) {
  const t = useT();
  const { data, isLoading, isError } = useMCPServerTools(server);

  const disabled = new Set(disabledTools);
  const autoApprove = new Set(autoApproveTools);

  const setDisabled = (name: string, isDisabled: boolean) => {
    const d = new Set(disabled);
    const a = new Set(autoApprove);
    if (isDisabled) {
      d.add(name);
      a.delete(name); // a hidden tool can't auto-approve — keep the lists coherent
    } else {
      d.delete(name);
    }
    onChange({ disabledTools: [...d], autoApproveTools: [...a] });
  };

  const setAutoApprove = (name: string, on: boolean) => {
    const a = new Set(autoApprove);
    if (on) {
      a.add(name);
    } else {
      a.delete(name);
    }
    onChange({ disabledTools: [...disabled], autoApproveTools: [...a] });
  };

  return (
    <div className="rounded-md bg-surface p-2.5">
      <div className="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-x-4 gap-y-1 px-1 pb-1.5 text-[10px] font-semibold text-fg-faint">
        <span>{t("mcp.tools.tool")}</span>
        <span className="w-12 text-center">{t("mcp.tools.enabled")}</span>
        <span className="w-12 text-center">{t("mcp.tools.autoApprove")}</span>
      </div>
      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{ icon: "tool", title: t("mcp.tools.empty") }}
      >
        {(tools) => (
          <div className="flex flex-col">
            {tools.map((tool) => {
              const isDisabled = disabled.has(tool.name);
              return (
                <div
                  key={tool.name}
                  className="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-x-4 rounded-sm px-1 py-1 hover:bg-surface-2"
                >
                  <code
                    className="truncate font-mono text-[12px] text-fg"
                    title={tool.description || tool.name}
                  >
                    {tool.name}
                  </code>
                  <div className="flex w-12 justify-center">
                    <Switch
                      checked={!isDisabled}
                      onCheckedChange={(on) => setDisabled(tool.name, !on)}
                      ariaLabel={t("mcp.tools.enable.aria", { tool: tool.name })}
                    />
                  </div>
                  <div className="flex w-12 justify-center">
                    <Switch
                      checked={autoApprove.has(tool.name)}
                      disabled={isDisabled}
                      onCheckedChange={(on) => setAutoApprove(tool.name, on)}
                      ariaLabel={t("mcp.tools.autoApprove.aria", { tool: tool.name })}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </DataView>
    </div>
  );
}
