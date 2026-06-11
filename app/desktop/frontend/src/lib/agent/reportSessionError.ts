// One reporting path for session-mutation failures (create / delete /
// rename / fork / relocate). Every mutation toasts on failure — a silent
// console.error reads as "the click did nothing" in the UI — while the
// console keeps the raw error object for diagnostics.

import { toast } from "sonner";

export function reportSessionError(action: string, err: unknown, description?: string): void {
  console.error(`[session] ${action} failed:`, err);
  toast.error(`Couldn't ${action} the session.`, description ? { description } : undefined);
}
