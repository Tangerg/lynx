// Client-minted message ids — the optimistic-bubble convention.
//
// The prefix and its two readers are one rule: the minter stamps an id a
// round-trip before the runtime streams the real Item, and the fold reconciles by
// recognizing it. They lived in the kernel's plugin-contract types, where nothing
// in the kernel used them and this context re-exported them through its own
// facade. The shape of the view state is a contract the kernel publishes; how
// this context labels a message it hasn't heard back about yet is not.

/** Optimistic (client-minted) user-message id prefix. send() stamps a bubble
 *  `${LOCAL_MESSAGE_PREFIX}${n}` a round-trip before the runtime streams the
 *  real userMessage Item, then the fold reconciles by matching this prefix.
 *  One owner for the convention so the minter (useAgentSession) and the
 *  matcher (agent fold) can't drift — change the prefix in one place
 *  and reconciliation would otherwise silently break (duplicate user bubble). */
export const LOCAL_MESSAGE_PREFIX = "local-";
export const isLocalMessageId = (id: string): boolean => id.startsWith(LOCAL_MESSAGE_PREFIX);

/** Optimistic id prefix for a STEER bubble (a message sent while a run is
 *  streaming, via runs.steer). Distinct from a plain send bubble because a
 *  steered message has NO id reconciler — runs.steer returns no userItemId
 *  (unlike send), so the fold can only reconcile it by content. A send bubble,
 *  by contrast, is relabeled to its server id before its Item streams, so it
 *  must never be matched by a steer item's content. */
export const LOCAL_STEER_PREFIX = `${LOCAL_MESSAGE_PREFIX}steer-`;
export const isLocalSteerMessageId = (id: string): boolean => id.startsWith(LOCAL_STEER_PREFIX);
