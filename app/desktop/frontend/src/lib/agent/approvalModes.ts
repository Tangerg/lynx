// The runtime's four approval stances, mirrored for the UI in backend order
// (approval.Mode). Single source of truth so the Approvals settings pane and
// the composer-side selector can't drift from each other or from the backend —
// adding a stance is a one-line change here. Labels/descriptions are i18n keys
// resolved by the consumer (module scope can't call the t() hook).

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
