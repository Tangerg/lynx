import { revalidateAgentSessionProjection } from "../session/refreshSessionProjection";

/** Re-read neutral Agent projection facts after an ambiguous/contended cancel.
 * The Adapter need not recognize any Runtime error variant: terminal is the
 * only fact that proves the cancellation objective has already been superseded. */
export async function revalidateRunTermination(sessionId: string, runId: string): Promise<boolean> {
  const result = await revalidateAgentSessionProjection(sessionId);
  return result?.authoritativeView.runsById[runId]?.status === "finished";
}
