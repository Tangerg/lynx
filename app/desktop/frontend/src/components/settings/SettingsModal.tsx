// Settings modal — built from a rail of plugin-contributed panes + a content
// pane. The first pane is selected by default.
//
// Esc-to-close used to live as a local useEffect here; it's now a plugin
// shortcut (`lyra.builtin.shortcuts` registers Escape → closeSettings).

import { useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Icon, type IconName } from "@/components/common";
import { swift, popIn } from "@/lib/motion";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { useSettingsPanes } from "@/plugins/sdk";

type Props = {
  open: boolean;
  onClose: () => void;
};

export function SettingsModal({ open, onClose }: Props) {
  const panes = useSettingsPanes();
  // `selectedId` is the user's explicit choice. If they haven't picked one
  // (or their pick has since been unregistered), we fall back to the first
  // pane via a *derived* value — no useEffect + setState chain that could
  // loop when panes changes references.
  const [selectedId, setSelectedId] = useState<string | undefined>();
  const activeId =
    selectedId && panes.some((p) => p.id === selectedId)
      ? selectedId
      : panes[0]?.id;

  const active = panes.find((p) => p.id === activeId);
  const ActiveBody = active?.component;

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="settings-backdrop"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={swift}
          onClick={onClose}
        >
          <motion.div
            className="settings-modal"
            {...popIn}
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
          >
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
                <button className="settings-close" onClick={onClose} title="Close (Esc)">
                  <Icon name="x" size={14} />
                </button>
              </div>
              <div className="settings-content-body">
                {ActiveBody && (
                  <PluginBoundary plugin={`settings:${active?.id ?? "unknown"}`}>
                    <ActiveBody />
                  </PluginBoundary>
                )}
              </div>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
