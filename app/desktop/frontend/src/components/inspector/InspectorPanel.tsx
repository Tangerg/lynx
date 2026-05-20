import { AnimatePresence, motion } from "motion/react";
import { Icon, Panel, type IconName } from "@/components/common";
import { slideRight } from "@/lib/motion";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { useInspectorTabs } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";
import { InspectorProvider } from "./InspectorContext";
import { InspectorRail, type RailBtn } from "./InspectorRail";

type Props = {
  open: boolean;
  tab: string;
  onTab: (t: string) => void;
  onClose: () => void;
  // Two tabs (diff + files) share the "currently-focused file" identifier;
  // others ignore it via the InspectorContext below.
  activeFile: string;
  onSelectFile: (path: string) => void;
};

/**
 * Pure router — discovers tabs from the plugin registry, renders the rail
 * for selection, animates between contents.
 *
 * Tabs read their own data via stores / queries; this component only owns
 * the open/closed + active-tab state plumbing.
 */
export function InspectorPanel({
  open, tab, onTab, onClose, activeFile, onSelectFile,
}: Props) {
  const tabs = useInspectorTabs();

  const buttons: RailBtn[] = tabs.map((spec) => ({
    id: spec.id,
    icon: spec.icon as IconName,
    label: spec.label,
    useBadge: spec.useBadge,
  }));

  const handleRailClick = (id: string) => {
    if (open && tab === id) onClose();
    else onTab(id);
  };

  const active = tabs.find((t) => t.id === tab) ?? tabs[0];
  const ActiveBody = active?.component;

  // "Open in main" promotes the current inspector tab to a workspace
  // view tab in the chat-area strip. Reading `useUIStore.openMainView`
  // direct keeps the inspector decoupled from the host SDK plumbing.
  const promoteActive = () => {
    if (!active) return;
    useUIStore.getState().openMainView({
      id: active.id,
      title: active.label,
      icon: active.icon,
    });
  };

  return (
    <Panel className={`inspector inspector-sheet ${open ? "open" : "closed"}`}>
      <InspectorRail
        open={open}
        tab={tab}
        buttons={buttons}
        onClick={handleRailClick}
        onClose={onClose}
      />

      <InspectorProvider value={{ activeFile, onSelectFile, onSwitchTab: onTab }}>
        <div className="insp-content">
          {active && (
            <button
              className="insp-promote"
              title={`Open ${active.label} in main`}
              onClick={promoteActive}
            >
              <Icon name="panel" size={12} />
              <span>Open in main</span>
            </button>
          )}
          <AnimatePresence mode="wait">
            {active && (
              <motion.div key={active.id} {...slideRight} style={{ display: "contents" }}>
                {ActiveBody && (
                  <PluginBoundary plugin={`inspector:${active.id}`} label={`${active.label} tab`}>
                    <ActiveBody />
                  </PluginBoundary>
                )}
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      </InspectorProvider>
    </Panel>
  );
}

// Re-export the hook so existing `from "@/components/inspector/InspectorPanel"`
// imports inside inspector tabs keep working without churn.
export { useInspector } from "./InspectorContext";
export type { InspectorContextValue } from "./InspectorContext";
