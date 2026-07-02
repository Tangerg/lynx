// The runtime's approval stances in backend order. Labels/descriptions are i18n
// keys resolved by consumers.

import type { ApprovalModeValue } from "@/lib/data/queries";

export interface ApprovalModeOption {
  value: ApprovalModeValue;
  /** i18n key for the short label (e.g. "Plan"). */
  labelKey: string;
  /** i18n key for the one-line description shown in the quick-switch menu. */
  descKey: string;
}

export const APPROVAL_MODES: ApprovalModeOption[] = [
  { value: "plan", labelKey: "approvals.mode.plan", descKey: "approvals.mode.plan.desc" },
  { value: "safe", labelKey: "approvals.mode.safe", descKey: "approvals.mode.safe.desc" },
  {
    value: "balanced",
    labelKey: "approvals.mode.balanced",
    descKey: "approvals.mode.balanced.desc",
  },
  { value: "yolo", labelKey: "approvals.mode.auto", descKey: "approvals.mode.auto.desc" },
];

// Fallback stance when a persisted / fetched mode doesn't resolve to a known
// option (union ↔ array drift). Referenced by value so it never couples to
// array position; if "balanced" is ever removed this throws at load rather than
// silently pointing at whatever now sits at that index.
export const DEFAULT_APPROVAL_MODE: ApprovalModeOption = APPROVAL_MODES.find(
  (m) => m.value === "balanced",
)!;
