// SettingsPage — the workspace view for app settings. Two-pane layout:
// a rail of plugin-registered panes on the left, the active pane on the
// right. Opens via Cmd+K → "View: Settings" or the sidebar-footer cog.

import { useState } from "react";
import { Icon, type IconName } from "@/components/common";
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
    <div className="settings-page">
      <div className="settings-rail">
        <div className="settings-rail-title">Settings</div>
        {panes.map((p) => (
          <button
            key={p.id}
            className={`settings-rail-btn ${p.id === activeId ? "active" : ""}`}
            onClick={() => setSelectedId(p.id)}
          >
            {p.icon && <Icon name={p.icon as IconName} size={14} />}
            <span>{p.label}</span>
          </button>
        ))}
      </div>
      <div className="settings-content">
        <div className="settings-content-head">
          <span className="settings-content-title">{active?.label ?? "Settings"}</span>
        </div>
        <div className="settings-content-body">
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
