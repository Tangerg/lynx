import { describeProblem } from "@/lib/rpcErrors";
import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type { MCPServerSettings } from "@/plugins/builtin/settings/mcp-servers/public/queries";
import type {
  WorkspaceFileChange as WorkspaceFileChangeSummary,
  WorkspaceProjectSummary,
} from "@/plugins/builtin/workspace/public/queries";
import type {
  McpServer as RpcMCPServer,
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
    model: session.model,
    cwd: session.workspace.ref.path,
    ...(session.workspace.availability === "missing" ? { cwdMissing: true } : {}),
    ...(session.favorite !== undefined ? { favorite: session.favorite } : {}),
    time: session.updatedAt || session.createdAt,
  };
}

const MCP_ICON: Record<string, string> = {
  Filesystem: "folder",
  Git: "branch",
  Shell: "terminal",
  "Web Search": "globe",
  Linear: "list",
  GitHub: "git",
  Postgres: "tool",
  Slack: "chat",
};

export function toMCPServerSettings(server: RpcMCPServer): MCPServerSettings {
  const connection = server.connection;
  const status = server.status;
  return {
    id: server.name,
    name: server.name,
    desc: server.description ?? "",
    tools: status.type === "connected" ? status.toolCount : 0,
    status: status.type,
    errorDetail: "error" in status ? describeProblem(status.error) : undefined,
    icon: MCP_ICON[server.name] ?? "tool",
    type: connection.type,
    enabled: status.type !== "disabled",
    description: server.description,
    url: connection.type === "streamableHttp" ? connection.url : undefined,
    authorizationMasked:
      connection.type === "streamableHttp" ? connection.authorizationMasked : undefined,
    headersMasked: connection.type === "streamableHttp" ? connection.headersMasked : undefined,
    command: connection.type === "stdio" ? connection.command : undefined,
    args: connection.type === "stdio" ? connection.args : undefined,
    envMasked: connection.type === "stdio" ? connection.envMasked : undefined,
    dir: connection.type === "stdio" ? connection.dir : undefined,
    timeoutSeconds: server.timeoutSeconds,
    disabledTools: server.disabledTools,
    autoApproveTools: server.autoApproveTools,
    toolCount: status.type === "connected" ? status.toolCount : undefined,
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
