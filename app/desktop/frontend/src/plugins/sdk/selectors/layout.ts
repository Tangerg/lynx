// UI-surface selectors — layout slots, Work Index items, workspace views and
// settings panes.

import { useMemo } from "react";
import type {
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
import { useContributions } from "../kernel";
import { createPointSubIndex, useExtensionPoint } from "./extensions";

const layoutBySlot = createPointSubIndex((item: { slot: string; spec: LayoutSlotSpec }) => ({
  key: item.slot,
  value: item.spec,
}));

export function useLayoutSlot(slot: string): LayoutSlotSpec[] {
  const entries = useContributions(LAYOUT_SLOT);
  return useMemo(
    () =>
      [...(layoutBySlot(entries).get(slot) ?? [])].sort(
        (a, b) => (a.order ?? 100) - (b.order ?? 100),
      ),
    [entries, slot],
  );
}

export function useWorkspaceViews(): WorkspaceViewSpec[] {
  return useExtensionPoint(WORKSPACE_VIEW);
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

export function useSettingsPanes(): SettingsPaneSpec[] {
  return useExtensionPoint(SETTINGS_PANE);
}
