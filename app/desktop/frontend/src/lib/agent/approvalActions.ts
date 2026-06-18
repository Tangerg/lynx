// Bridge from the card-less keyboard approve/decline path (⌘↩ / ⇧⌘⌫, see
// submitPendingApproval) to the mounted ApprovalCard's own submit — so the
// shortcut applies the card's edited args + "remember" checkbox exactly like
// clicking its buttons, instead of resuming a bare approval that drops both.
//
// The card registers stable approve/decline thunks (each reads the card's
// latest local state via a ref) keyed by the interrupt's itemId while it's
// actionable; submitPendingApproval looks them up before falling back to a
// bare resume (the card not being mounted is the only fallback case).

export interface ApprovalActions {
  approve: () => void;
  decline: () => void;
}

const registry = new Map<string, ApprovalActions>();

/** Register a pending approval card's submit thunks; returns the unregister. */
export function registerApprovalActions(itemId: string, actions: ApprovalActions): () => void {
  registry.set(itemId, actions);
  return () => {
    // Only delete if still ours — a fast remount could have re-registered.
    if (registry.get(itemId) === actions) registry.delete(itemId);
  };
}

/** The mounted card's submit thunks for itemId, or undefined when none. */
export function getApprovalActions(itemId: string): ApprovalActions | undefined {
  return registry.get(itemId);
}
