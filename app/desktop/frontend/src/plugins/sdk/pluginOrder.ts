import type { PluginSpec } from "./types";

interface Skipped {
  name: string;
  reason: string;
}

/**
 * Kahn-style topological sort. Picks the ready node with the lowest input index
 * so manifest order survives as a tie-breaker.
 */
export function orderPlugins(specs: PluginSpec[]): { order: PluginSpec[]; skipped: Skipped[] } {
  const byName = new Map(specs.map((s) => [s.name, s]));
  const index = new Map(specs.map((s, i) => [s.name, i]));
  const skipped: Skipped[] = [];

  const requires = new Map<string, string[]>();
  for (const s of specs) {
    const deps = (s.requires ?? []).filter((dep) => {
      if (!byName.has(dep)) {
        skipped.push({
          name: s.name,
          reason: `requires "${dep}" which is not loaded`,
        });
        return false;
      }
      return true;
    });
    requires.set(s.name, deps);
  }
  const skippedNames = new Set(skipped.map((s) => s.name));

  // Without transitive propagation, B -> A -> missing would drop A but still
  // load B without its declared dependency.
  let propagated = true;
  while (propagated) {
    propagated = false;
    for (const s of specs) {
      if (skippedNames.has(s.name)) continue;
      const dead = requires.get(s.name)!.find((dep) => skippedNames.has(dep));
      if (dead !== undefined) {
        skipped.push({ name: s.name, reason: `requires "${dead}" which was skipped` });
        skippedNames.add(s.name);
        propagated = true;
      }
    }
  }

  const inDegree = new Map<string, number>();
  const dependents = new Map<string, string[]>();
  for (const s of specs) {
    if (skippedNames.has(s.name)) continue;
    inDegree.set(s.name, 0);
    dependents.set(s.name, []);
  }
  for (const s of specs) {
    if (skippedNames.has(s.name)) continue;
    // Post-fixpoint, every surviving edge points at a live dependency.
    for (const dep of requires.get(s.name)!) {
      inDegree.set(s.name, (inDegree.get(s.name) ?? 0) + 1);
      dependents.get(dep)!.push(s.name);
    }
  }

  const ready = new Set<string>();
  for (const [name, deg] of inDegree) if (deg === 0) ready.add(name);

  const order: PluginSpec[] = [];
  while (ready.size > 0) {
    let pick: string | null = null;
    let pickIdx = Infinity;
    for (const name of ready) {
      const i = index.get(name)!;
      if (i < pickIdx) {
        pick = name;
        pickIdx = i;
      }
    }
    ready.delete(pick!);
    order.push(byName.get(pick!)!);
    for (const child of dependents.get(pick!)!) {
      const next = (inDegree.get(child) ?? 0) - 1;
      inDegree.set(child, next);
      if (next === 0) ready.add(child);
    }
  }

  for (const [name, deg] of inDegree) {
    if (deg > 0) {
      skipped.push({ name, reason: "dependency cycle (skipped)" });
    }
  }

  return { order, skipped };
}
