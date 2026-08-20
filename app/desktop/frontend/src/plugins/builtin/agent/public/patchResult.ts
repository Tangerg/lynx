/** One filesystem mutation from Runtime `PatchResult.changes`. */
export interface PatchChange {
  path: string;
  status: "added" | "deleted" | "modified" | "moved";
  from?: string;
}

const PATCH_STATUSES = new Set<PatchChange["status"]>(["added", "deleted", "modified", "moved"]);

/**
 * Parse the persisted result of one `apply_patch` ToolCall.
 *
 * This is shared Agent language because both the central Narrative and the
 * right-side Run Summary consume the same durable receipt. Invalid or legacy
 * shapes produce no facts; callers must not replace them with current worktree
 * state, which belongs to a different scope and point in time.
 */
export function projectPatchChanges(result: string | undefined): PatchChange[] {
  const changes = jsonRecord(result)?.changes;
  if (!Array.isArray(changes)) return [];
  return changes.flatMap((value): PatchChange[] => {
    const change = record(value);
    const path = text(change.path);
    const status = text(change.status) as PatchChange["status"];
    if (!path || !PATCH_STATUSES.has(status)) return [];
    const from = text(change.from);
    return [{ path, status, ...(status === "moved" && from ? { from } : {}) }];
  });
}

function jsonRecord(value: string | undefined): Record<string, unknown> | undefined {
  if (!value) return undefined;
  try {
    return record(JSON.parse(value));
  } catch {
    return undefined;
  }
}

function record(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}
