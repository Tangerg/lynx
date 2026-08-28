import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type {
  WorkspaceFileChange as WorkspaceFileChangeSummary,
  WorkspaceProjectSummary,
} from "@/plugins/builtin/workspace/public/queries";
import type {
  Session,
  WorkspaceFileChange as RpcFileChange,
  WorkspaceSummary as RpcWorkspaceSummary,
} from "@/rpc";
import { isErrorType } from "@/rpc";

export function toAgentSessionSummary(session: Session): AgentSessionSummary {
  return {
    id: session.id,
    revision: session.revision,
    title: session.title,
    status: session.status,
    provider: session.provider,
    model: session.model,
    ...(session.reasoningEffort ? { reasoningEffort: session.reasoningEffort } : {}),
    workspace: {
      path: session.workspace.ref.path,
      availability: session.workspace.availability,
    },
    ...(session.favorite !== undefined ? { favorite: session.favorite } : {}),
    time: session.updatedAt || session.createdAt,
  };
}

export function toWorkspaceProjectSummary(summary: RpcWorkspaceSummary): WorkspaceProjectSummary {
  return {
    id: summary.workspace.ref.path,
    name: summary.name,
    sessionCount: summary.sessionCount,
    ...(summary.workspace.availability === "missing" ? { cwdMissing: true } : {}),
  };
}

const FILE_CHANGE: Record<RpcFileChange["status"], WorkspaceFileChangeSummary["change"]> = {
  added: "add",
  untracked: "add",
  modified: "mod",
  renamed: "mod",
  deleted: "del",
};

export function toWorkspaceFileChangeSummary(change: RpcFileChange): WorkspaceFileChangeSummary {
  return {
    path: change.path,
    change: FILE_CHANGE[change.status],
    added: change.added,
    removed: change.removed,
    binary: change.binary,
  };
}

// Capability-gated workspace reads should render as empty optional surfaces,
// not as broken panes, when the runtime negotiated the feature off.
export function emptyListIfUngated(error: unknown): never[] {
  if (isErrorType(error, "capability_not_negotiated")) return [];
  throw error;
}
