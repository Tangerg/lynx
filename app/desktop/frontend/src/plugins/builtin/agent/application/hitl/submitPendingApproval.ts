// Imperative HITL approval submit — the keyboard path behind ⌘↩ (approve) and
// ⇧⌘⌫ (deny) in the composer keymap. The card path goes through
// useApprovalSubmit / useInterruptResume (per-card optimistic state); this is
// the card-less equivalent: find the active session's first unstaged approval
// and add its answer to the shared atomic response set.
//
// Returns true when an approval is pending or staged (so the keybinding never
// falls through into chat send while the barrier is open), false when the set
// contains no approval. The coordinator owns staging, deduplication and rollback.

import { agentSessionState } from "../ports/sessionState";
import { agentSessionView } from "../ports/sessionView";
import { getApprovalActions } from "./useApprovalSubmit";
import type { ApprovalDecision } from "../../domain/hitl";
import { WIRE_DECISION } from "./wireDecision";
import { resumeInterrupt } from "./useInterruptResume";
import { interruptResponseIsStaged } from "./interruptResponseCoordinator";

export function submitPendingApproval(decision: ApprovalDecision): boolean {
  const sid = agentSessionState().getActiveSessionId();
  const entry = agentSessionView().getSession(sid);
  if (!entry) return false;

  // Questions need answers (not approve/deny), so only act on approval interrupts.
  const hasPendingApproval = entry.view.pendingInterrupts.some((group) =>
    group.interrupts.some((interrupt) => interrupt.kind === "approval"),
  );
  const oi = entry.view.pendingInterrupts.find((group) =>
    group.interrupts.some(
      (interrupt) =>
        interrupt.kind === "approval" &&
        !interruptResponseIsStaged(sid, group.rootRunId, interrupt.itemId),
    ),
  );
  const interrupt = oi?.interrupts.find(
    (candidate) =>
      candidate.kind === "approval" &&
      !interruptResponseIsStaged(sid, oi.rootRunId, candidate.itemId),
  );
  // Every approval in the atomic set is already staged or submitting. Consume
  // a repeated shortcut instead of letting it fall through into chat send.
  if (!oi || !interrupt) return hasPendingApproval;

  const itemId = interrupt.itemId;
  // Prefer the mounted card's own submit so the shortcut applies its edited
  // args + remember exactly like its buttons. Direct staging below is only for
  // the no-card-mounted fallback.
  const actions = getApprovalActions(sid, oi.rootRunId, itemId);
  if (actions) {
    if (decision === "approved") actions.approve();
    else actions.decline();
    return true;
  }

  resumeInterrupt(
    sid,
    oi.rootRunId,
    itemId,
    { type: "approval", decision: WIRE_DECISION[decision] },
    { decision },
  );
  return true;
}
