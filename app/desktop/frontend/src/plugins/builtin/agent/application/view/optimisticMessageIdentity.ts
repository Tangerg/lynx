// A start ack names its opening Item and is reconciled by exact id. Steer has no
// Item id in its ack, so only steer placeholders use content reconciliation.
export const OPTIMISTIC_USER_MESSAGE_PREFIX = "local-";
export const OPTIMISTIC_STEER_MESSAGE_PREFIX = `${OPTIMISTIC_USER_MESSAGE_PREFIX}steer-`;

export function isOptimisticSteerMessageId(id: string): boolean {
  return id.startsWith(OPTIMISTIC_STEER_MESSAGE_PREFIX);
}
