import type { ReactNode } from "react";
import { AgentContextDock } from "@/ui/agent";

/**
 * Retires view-local state at the exact Session ownership boundary.
 *
 * Dock navigation is persisted per Session, but the mounted view subtree also
 * owns transient state such as scroll anchoring and collapsed file sections.
 * Keying the structural dock keeps that state alive while browsing one Session
 * and prevents React from handing it to another Session with the same view IDs.
 */
export function SessionOwnedDock({
  sessionId,
  children,
}: {
  sessionId: string;
  children: ReactNode;
}) {
  return <AgentContextDock key={sessionId}>{children}</AgentContextDock>;
}
