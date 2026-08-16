import type { LayoutSlotSpec } from "@/plugins/sdk";

/** After the model it measures — the window belongs to the model, so it reads
 *  as that control's consequence rather than as another setting. */
export function composerContextUsageSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return { id: "context-usage", order: 3, component };
}
