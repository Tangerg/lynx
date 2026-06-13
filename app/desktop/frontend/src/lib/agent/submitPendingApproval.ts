// Imperative HITL approval submit — the keyboard path behind ⌘↩ (approve) and
// ⇧⌘⌫ (deny) in the composer keymap. The card path goes through
// useApprovalSubmit / useInterruptResume (per-card optimistic state); this is
// the card-less equivalent: answer the active session's first OPEN approval
// interrupt directly.
//
// Returns true when it found and submitted an approval (so the keybinding
// consumes the event), false when none is pending (so ⌘↩ falls through to
// send). resolveInterrupt is deferred to run-started exactly like
// useInterruptResume, so a rejected runs.resume leaves the interrupt intact +
// retryable; a module-level in-flight set guards that window against a double
// press (the second press finds the same open interrupt before it settles).

import { asItemId, asRunId } from "@/rpc";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";
import { type ApprovalDecision, WIRE_DECISION } from "./hitlDecision";

const inFlight = new Set<string>();

export function submitPendingApproval(decision: ApprovalDecision): boolean {
  const sid = useSessionStore.getState().activeSessionId;
  const entry = useAgentStore.getState().sessions[sid];
  const resume = entry?.resume;
  if (!entry || !resume) return false;

  // Questions need answers (not approve/deny), so only act on approval interrupts.
  const oi = entry.view.openInterrupts.find((o) => o.interrupts.some((i) => i.type === "approval"));
  const interrupt = oi?.interrupts.find((i) => i.type === "approval");
  if (!oi || !interrupt) return false;

  const itemId = interrupt.itemId;
  if (inFlight.has(itemId)) return true; // already submitting — swallow the repeat press
  inFlight.add(itemId);
  resume(
    asRunId(oi.parentRunId),
    [
      {
        itemId: asItemId(itemId),
        response: { type: "approval", decision: WIRE_DECISION[decision] },
      },
    ],
    () => {
      useAgentStore.getState().resolveInterrupt(sid, itemId, { decision });
      inFlight.delete(itemId);
    },
    () => inFlight.delete(itemId), // channel-a failure → leave it retryable
  );
  return true;
}
