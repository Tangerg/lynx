// SettingsPage — the workspace view for app settings. Two-pane layout:
// a rail of plugin-registered panes on the left, the active pane on the
// right. Opens via Cmd+K → "View: Settings" or the sidebar-footer cog.

import { useState } from "react";
import { Icon, type IconName } from "@/components/common";
import { cn } from "@/lib/utils";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { useSettingsPanes } from "@/plugins/sdk";

export function SettingsPage() {
  const panes = useSettingsPanes();
  // `selectedId` is the user's explicit choice. If they haven't picked
  // one (or their pick has since been unregistered), fall back to the
  // first pane via a derived value — no useEffect/setState loop.
  const [selectedId, setSelectedId] = useState<string | undefined>();
  const activeId =
    selectedId && panes.some((p) => p.id === selectedId)
      ? selectedId
      : panes[0]?.id;

  const active = panes.find((p) => p.id === activeId);
  const ActiveBody = active?.component;

  return (
    <div className="grid h-full w-full grid-cols-[200px_1fr] overflow-hidden">
      <div className="flex flex-col gap-0.5 bg-surface-2 px-2 py-3.5">
        <div className="px-2.5 pb-2 pt-1 font-mono text-[10.5px] font-semibold text-fg-faint">
          Settings
        </div>
        {panes.map((p) => (
          <button
            key={p.id}
            type="button"
            onClick={() => setSelectedId(p.id)}
            className={cn(
              "flex items-center gap-2 rounded-md border-0 bg-transparent px-2.5 py-1.5 text-left text-[13px] text-fg-muted cursor-pointer transition-colors duration-150 hover:bg-[color-mix(in_srgb,var(--color-text)_5%,transparent)] hover:text-fg",
              p.id === activeId && "bg-surface-3 text-fg",
            )}
          >
            {p.icon && <Icon name={p.icon as IconName} size={14} />}
            <span>{p.label}</span>
          </button>
        ))}
      </div>
      <div className="flex min-h-0 min-w-0 flex-col">
        <div className="flex items-center justify-between px-4.5 py-3.5">
          <span className="text-[16px] font-semibold">{active?.label ?? "Settings"}</span>
        </div>
        <div className="flex-1 min-h-0 overflow-y-auto px-5 py-4.5">
          {ActiveBody && (
            <PluginBoundary plugin={`settings:${active?.id ?? "unknown"}`}>
              <ActiveBody />
            </PluginBoundary>
          )}
        </div>
      </div>
    </div>
  );
}
