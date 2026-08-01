import { describeProblem } from "@/lib/rpcErrors";
import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type {
  MCPServerSummary,
  MCPServerSettings,
} from "@/plugins/builtin/settings/mcp-servers/public/queries";
import type {
  WorkspaceFileChange as WorkspaceFileChangeSummary,
  WorkspaceProjectSummary,
} from "@/plugins/builtin/workspace/public/queries";
import type {
  McpServer as RpcMCPServer,
  McpServerConfig as RpcMCPServerConfig,
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

export function toMCPServerSummary(server: RpcMCPServer): MCPServerSummary {
  return {
    id: server.name,
    name: server.name,
    desc: server.description ?? "",
    tools: server.toolCount ?? 0,
    status: server.status,
    errorDetail: describeProblem(server.error),
    icon: MCP_ICON[server.name] ?? "tool",
  };
}

export function toMCPServerSettings(
  configuration: RpcMCPServerConfig,
  live?: RpcMCPServer,
): MCPServerSettings {
  return {
    name: configuration.name,
    type: configuration.type,
    enabled: configuration.enabled,
    description: configuration.description,
    url: configuration.url,
    authorizationMasked: configuration.authorizationMasked,
    command: configuration.command,
    args: configuration.args,
    env: configuration.env,
    dir: configuration.dir,
    disabledTools: configuration.disabledTools,
    autoApproveTools: configuration.autoApproveTools,
    status: live?.status,
    toolCount: live?.toolCount,
    errorDetail: describeProblem(live?.error),
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
export function emptyPageIfUngated(error: unknown): { data: never[] } {
  if (isErrorType(error, "capability_not_negotiated")) return { data: [] };
  throw error;
}
