import type { RuntimeServiceSnapshot } from "./ports/serviceStatus";

export function runtimeServiceAcceptsCommands(snapshot: RuntimeServiceSnapshot): boolean {
  return snapshot.observation !== null && snapshot.phase !== "unavailable";
}
