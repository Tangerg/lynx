import { useCallback } from "react";
import { invalidateSessions } from "@/lib/data/queries";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { reportSessionError } from "./reportSessionError";

// In-flight forks keyed by target (session id + optional root run). A fork is a
// full round-trip; without this latch a double-click — or a context-menu item
// that re-fires — would mint two duplicate forks. Re-entrant calls for the same
// target join the pending fork; different targets still run concurrently.
// Mirrors useCreateSession's inflight guard.
const inflight = new Map<string, Promise<void>>();

/** Imperative fork for non-React callers (message context-menu actions).
 *  `fromRunId` = branch up to AND INCLUDING that root run (AUX_API §4.2);
 *  omitted = whole-session copy. The fork inherits the source's chat history,
 *  so unlike a fresh create it is no draft — it shows in the sidebar
 *  immediately, and we open its tab. */
export function forkSessionAt(id: string, fromRunId?: string): Promise<void> {
  const key = fromRunId ? `${id}:${fromRunId}` : id;
  const pending = inflight.get(key);
  if (pending) return pending;
  const run = doFork(id, fromRunId).finally(() => inflight.delete(key));
  inflight.set(key, run);
  return run;
}

async function doFork(id: string, fromRunId?: string): Promise<void> {
  try {
    const fork = await agentRuntime().forkSession({ sessionId: id, fromRunId });
    agentSessionState().selectSession(fork.id);
    void invalidateSessions();
  } catch (err) {
    reportSessionError("fork", err);
  }
}

export function useForkSession(): (id: string) => Promise<void> {
  // Stable identity for React callers; the imperative core owns the logic.
  return useCallback((id) => forkSessionAt(id), []);
}
