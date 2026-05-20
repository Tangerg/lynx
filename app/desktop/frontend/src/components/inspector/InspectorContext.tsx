// Shared context for inspector tabs.
//
// Originally co-located with InspectorPanel, but the "open in main"
// affordance needs the same context available when an inspector tab's
// body renders inside ChatPanel. Hoisting the provider/hook into its
// own module lets both call sites wrap with it.

import { createContext, useContext, type ReactNode } from "react";
import { useUIStore } from "@/state/uiStore";

export type InspectorContextValue = {
  activeFile: string;
  onSelectFile: (path: string) => void;
  onSwitchTab: (id: string) => void;
};

const InspectorContext = createContext<InspectorContextValue | null>(null);

/**
 * Provider wrapper. Both InspectorPanel and the chat-area "main view"
 * (when an inspector tab has been promoted there) render their tab body
 * inside this.
 */
export function InspectorProvider({
  value,
  children,
}: {
  value: InspectorContextValue;
  children: ReactNode;
}) {
  return <InspectorContext.Provider value={value}>{children}</InspectorContext.Provider>;
}

/**
 * Read the inspector context. Falls back to a default that reads / writes
 * `useUIStore` directly so an inspector tab promoted into a main view
 * works without anyone setting up a provider — clicking a row that calls
 * `onSelectFile` mutates the same UI store as the docked inspector does.
 *
 * Both hooks below are called *unconditionally* so React's hook-order
 * invariant holds whether the caller is wrapped in an InspectorProvider
 * or not. The branch is at return time, not at hook-call time.
 */
export function useInspector(): InspectorContextValue {
  const ctx = useContext(InspectorContext);
  const fallbackFile = useUIStore((s) => s.activeFile);
  const fallbackSetFile = useUIStore((s) => s.setActiveFile);
  const fallbackSetTab = useUIStore((s) => s.setInspectorTab);
  return ctx ?? {
    activeFile: fallbackFile,
    onSelectFile: fallbackSetFile,
    onSwitchTab: fallbackSetTab,
  };
}
