// SettingsPage — the full-width "open as a main-area tab" variant of the
// settings UI. Replaced the modal (`SettingsModal`) so settings reads /
// edits like any other workspace view: navigate to it via Cmd+K → "View:
// Settings", close the tab when you're done.
//
// Internal layout is the same two-pane rail + content split the modal
// had; the chrome (backdrop, motion, close button) is gone because the
// chat-area tab strip already handles open/close.

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
