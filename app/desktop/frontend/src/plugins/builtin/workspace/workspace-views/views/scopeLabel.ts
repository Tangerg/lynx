import type { MemoryEntryInfo } from "@/lib/data/queries";

// Display labels for a memory/agent-doc scope (cwd / projectRoot / home). The
// Memory and Agent-docs views surface the same scope domain, so the labels live
// here once and can't drift.
const SCOPE_LABEL: Record<MemoryEntryInfo["scope"], string> = {
  cwd: "cwd",
  projectRoot: "project root",
  home: "home",
};

/** Scope label, falling back to the raw value for scopes outside the known set. */
export function scopeLabel(scope: string): string {
  return (SCOPE_LABEL as Record<string, string>)[scope] ?? scope;
}
