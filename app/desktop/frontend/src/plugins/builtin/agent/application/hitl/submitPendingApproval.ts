// Imperative HITL approval submit — the keyboard path behind ⌘↩ (approve) and
// ⇧⌘⌫ (deny) in the composer keymap. The card path goes through
// useApprovalSubmit / useInterruptResume (per-card optimistic state); this is
// the card-less equivalent: find the active session's first OPEN approval
// interrupt and answer it via the shared resumeInterrupt core (same
// deferred-settle semantic).
//
// Returns true when it found and submitted an approval (so the keybinding
// consumes the event), false when none is pending (so ⌘↩ falls through to
// send). A module-level in-flight set guards the deferred-settle window against
// a double press (the second press finds the same open interrupt before it
// settles); cleared on settle/error so a torn-down session stays retryable.

import { useAgentStore } from "@/state/agentStore";
import { useAgentSessionStore } from "@/state/agentSessionStore";
import { getApprovalActions } from "./approvalActions";
import type { ApprovalDecision } from "../../domain/hitl";
import { WIRE_DECISION } from "./wireDecision";
import { resumeInterrupt } from "./useInterruptResume";

const inFlight = new Set<string>();

export function submitPendingApproval(decision: ApprovalDecision): boolean {
  const sid = useAgentSessionStore.getState().activeSessionId;
  const entry = useAgentStore.getState().sessions[sid];
  if (!entry) return false;

  // Questions need answers (not approve/deny), so only act on approval interrupts.
  const oi = entry.view.openInterrupts.find((o) => o.interrupts.some((i) => i.type === "approval"));
  const interrupt = oi?.interrupts.find((i) => i.type === "approval");
  if (!oi || !interrupt) return false;

  const itemId = interrupt.itemId;
  // Prefer the mounted card's own submit so the shortcut applies its edited
  // args + remember exactly like its buttons (its optimistic settle removes the
  // open interrupt, so a repeat press finds nothing and falls through). Bare
  // resume below is only for the no-card-mounted fallback.
  const actions = getApprovalActions(itemId);
  if (actions) {
    if (decision === "approved") actions.approve();
    else actions.decline();
    return true;
  }

  if (inFlight.has(itemId)) return true; // already submitting — swallow the repeat press
  inFlight.add(itemId);
  const clear = () => inFlight.delete(itemId);
  if (
    !resumeInterrupt(
      sid,
      oi.parentRunId,
      itemId,
      { type: "approval", decision: WIRE_DECISION[decision] },
      { decision },
      { onSettled: clear, onError: clear },
    )
  ) {
    clear(); // no resume binding (rare race) — don't leak the latch
  }
  return true;
}
