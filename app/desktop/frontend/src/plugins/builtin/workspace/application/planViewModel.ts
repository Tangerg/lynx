import type { Translate } from "@/lib/i18n";
import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";

export interface PlanViewModel {
  items: readonly PlanItem[];
  doneCount: number;
  totalCount: number;
  isEmpty: boolean;
}

export function planViewModel(items: readonly PlanItem[]): PlanViewModel {
  let doneCount = 0;
  for (const item of items) {
    if (item.status === "done") {
      doneCount += 1;
    }
  }

  return {
    items,
    doneCount,
    totalCount: items.length,
    isEmpty: items.length === 0,
  };
}

export function planSubtext(
  t: Translate,
  { doneCount, totalCount }: Pick<PlanViewModel, "doneCount" | "totalCount">,
): string | undefined {
  if (totalCount === 0) {
    return undefined;
  }
  return t("plan.complete", { done: doneCount, total: totalCount });
}
