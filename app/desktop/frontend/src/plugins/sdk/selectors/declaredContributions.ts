import { useMemo } from "react";
import type { Owned } from "../registryState";

interface OrderedContribution {
  order?: number;
}

/**
 * Resolve declared placeholders and registered contributions into one ordered
 * surface. A registered contribution replaces its declaration with the same id.
 */
export function useResolvedContributions<
  Declaration extends { id: string },
  Contribution extends { id: string } & OrderedContribution,
>(
  registered: Contribution[],
  declared: Map<string, Owned<Declaration>>,
  resolveDeclaration: (declaration: Declaration, pluginName: string) => Contribution,
): Contribution[] {
  return useMemo(() => {
    const contributionsById = new Map<string, Contribution>();
    for (const entry of declared.values()) {
      contributionsById.set(entry.value.id, resolveDeclaration(entry.value, entry.pluginName));
    }
    for (const contribution of registered) {
      contributionsById.set(contribution.id, contribution);
    }
    return Array.from(contributionsById.values()).sort(
      (left, right) => (left.order ?? 100) - (right.order ?? 100),
    );
  }, [registered, declared, resolveDeclaration]);
}
