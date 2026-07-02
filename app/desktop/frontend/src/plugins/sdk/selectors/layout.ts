// UI-surface selectors — layout slots, Work Index items, and
// the "registered + declared placeholder" merged surfaces (workspace
// views + settings panes).

import { useMemo } from "react";
import type {
  ContributedSettingsPane,
  ContributedView,
  ContextDockDestinationSpec,
  LayoutSlotSpec,
  SettingsPaneSpec,
  WorkIndexItemPlacement,
  WorkIndexItemSpec,
  WorkspaceViewSpec,
} from "../types";
import {
  CONTEXT_DOCK_DESTINATION,
  LAYOUT_SLOT,
  SETTINGS_PANE,
  WORK_INDEX_ITEM,
  WORKSPACE_VIEW,
} from "../kernelPoints";
import { makeLazyActivator } from "../lazyActivator";
import { usePluginStore } from "../registry";
import { runActivator, useDeclaredMerged } from "./_helpers";
import { createPointSubIndex, useExtensionPoint } from "./extensions";

const layoutBySlot = createPointSubIndex<{ slot: string; spec: LayoutSlotSpec }, LayoutSlotSpec>(
  LAYOUT_SLOT.id,
  (item) => ({ key: item.slot, value: item.spec }),
);

export function useLayoutSlot(slot: string): LayoutSlotSpec[] {
  const extensions = usePluginStore((s) => s.extensions);
  return useMemo(
    () =>
      [...(layoutBySlot(extensions).get(slot) ?? [])].sort(
        (a, b) => (a.order ?? 100) - (b.order ?? 100),
      ),
    [extensions, slot],
  );
}

export function useWorkspaceViews(): WorkspaceViewSpec[] {
  const registered = useExtensionPoint(WORKSPACE_VIEW);
  const declared = usePluginStore((s) => s.declaredViews);
  return useDeclaredMerged(registered, declared, declaredToWorkspaceView);
}

export function useContextDockDestinations(): ContextDockDestinationSpec[] {
  return useExtensionPoint(CONTEXT_DOCK_DESTINATION);
}

export function useWorkIndexItems(placement: WorkIndexItemPlacement): WorkIndexItemSpec[] {
  return useExtensionPoint(WORK_INDEX_ITEM).filter((item) => item.placement === placement);
}

function declaredToWorkspaceView(d: ContributedView, pluginName: string): WorkspaceViewSpec {
  return {
    ...d,
    component: makeLazyActivator(d.title, () => {
      void runActivator(pluginName);
    }),
  };
}

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
