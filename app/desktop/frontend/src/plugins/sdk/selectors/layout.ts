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
  WorkIndexItemScope,
  WorkIndexItemSpec,
  WorkIndexItemVariant,
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
import { useResolvedContributions } from "./declaredContributions";
import { createPointSubIndex, useExtensionPoint } from "./extensions";
import { activatePlugin } from "./pluginActivation";

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
  const declared = usePluginStore((state) => state.declaredViews);
  return useResolvedContributions(registered, declared, declaredToWorkspaceView);
}

export function useContextDockDestinations(): ContextDockDestinationSpec[] {
  return useExtensionPoint(CONTEXT_DOCK_DESTINATION);
}

export function useWorkIndexItems(
  variant: WorkIndexItemVariant,
  scope?: WorkIndexItemScope,
): WorkIndexItemSpec[] {
  const items = useExtensionPoint(WORK_INDEX_ITEM);
  return useMemo(
    () => items.filter((item) => item.variant === variant && (!scope || item.scope === scope)),
    [items, variant, scope],
  );
}

function declaredToWorkspaceView(view: ContributedView, pluginName: string): WorkspaceViewSpec {
  return {
    ...view,
    component: makeLazyActivator(view.title, () => {
      void activatePlugin(pluginName);
    }),
  };
}

export function useSettingsPanes(): SettingsPaneSpec[] {
  const registered = useExtensionPoint(SETTINGS_PANE);
  const declared = usePluginStore((state) => state.declaredSettingsPanes);
  return useResolvedContributions(registered, declared, declaredToSettingsPane);
}

function declaredToSettingsPane(
  pane: ContributedSettingsPane,
  pluginName: string,
): SettingsPaneSpec {
  return {
    ...pane,
    component: makeLazyActivator(pane.label, () => {
      void activatePlugin(pluginName);
    }),
  };
}
