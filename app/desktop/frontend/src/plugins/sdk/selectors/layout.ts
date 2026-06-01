// UI-surface selectors — layout slots, sidebar (sections + rail), and
// the "registered + declared placeholder" merged surfaces (workspace
// views + settings panes).

import { useMemo } from "react";
import type {
  ContributedSettingsPane,
  ContributedView,
  LayoutSlotSpec,
  SettingsPaneSpec,
  SidebarRailItemSpec,
  SidebarSectionSpec,
  WorkspaceViewSpec,
} from "../types";
import { SETTINGS_PANE, SIDEBAR_RAIL_ITEM, SIDEBAR_SECTION, WORKSPACE_VIEW } from "../kernelPoints";
import { makeLazyActivator } from "../lazyActivator";
import { usePluginStore } from "../registry";
import { createIndex, runActivator, useDeclaredMerged } from "./_helpers";
import { useExtensionPoint } from "./extensions";

// ---------------------------------------------------------------------------
// Layout slots
// ---------------------------------------------------------------------------

const layoutBySlot = createIndex<{ slot: string; spec: LayoutSlotSpec }, LayoutSlotSpec>((o) => ({
  key: o.value.slot,
  value: o.value.spec,
}));

export function useLayoutSlot(slot: string): LayoutSlotSpec[] {
  const map = usePluginStore((s) => s.layoutSlots);
  return useMemo(
    () =>
      [...(layoutBySlot(map).get(slot) ?? [])].sort((a, b) => (a.order ?? 100) - (b.order ?? 100)),
    [map, slot],
  );
}

// ---------------------------------------------------------------------------
// Sidebar
// ---------------------------------------------------------------------------

export function useSidebarSections(): SidebarSectionSpec[] {
  return useExtensionPoint(SIDEBAR_SECTION);
}

export function useSidebarRailItems(): SidebarRailItemSpec[] {
  return useExtensionPoint(SIDEBAR_RAIL_ITEM);
}

// ---------------------------------------------------------------------------
// Workspace views (registered + declared merged)
// ---------------------------------------------------------------------------

export function useWorkspaceViews(): WorkspaceViewSpec[] {
  const registered = useExtensionPoint(WORKSPACE_VIEW);
  const declared = usePluginStore((s) => s.declaredViews);
  return useDeclaredMerged(registered, declared, declaredToWorkspaceView);
}

function declaredToWorkspaceView(d: ContributedView, pluginName: string): WorkspaceViewSpec {
  return {
    ...d,
    component: makeLazyActivator(d.title, () => {
      void runActivator(pluginName);
    }),
  };
}

// ---------------------------------------------------------------------------
// Settings panes (registered + declared merged)
// ---------------------------------------------------------------------------

export function useSettingsPanes(): SettingsPaneSpec[] {
  const registered = useExtensionPoint(SETTINGS_PANE);
  const declared = usePluginStore((s) => s.declaredSettingsPanes);
  return useDeclaredMerged(registered, declared, declaredToSettingsPane);
}

function declaredToSettingsPane(d: ContributedSettingsPane, pluginName: string): SettingsPaneSpec {
  return {
    ...d,
    component: makeLazyActivator(d.label, () => {
      void runActivator(pluginName);
    }),
  };
}
