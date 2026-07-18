import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type {
  MCPServer as McpServerStatusSummary,
  MCPServerConfigInfo,
} from "@/plugins/builtin/settings/mcp-servers/public/data";
import type {
  WorkspaceFileChange as WorkspaceFileChangeSummary,
  WorkspaceProjectSummary,
} from "@/plugins/builtin/workspace/public/data";
import type {
  McpServer as RpcMCPServer,
  McpServerConfig as RpcMCPServerConfig,
  Project as RpcProject,
  Session,
  WorkspaceFileChange as RpcFileChange,
} from "@/rpc";
import { isErrorType } from "@/rpc";

export function toAgentSessionSummary(s: Session): AgentSessionSummary {
  return {
    id: s.id,
    title: s.title,
    status: s.status,
    model: s.model,
    cwd: s.cwd,
    cwdMissing: s.cwdMissing,
    favorite: s.favorite,
    time: s.updatedAt || s.createdAt,
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

export function toMcpServerStatusSummary(s: RpcMCPServer): McpServerStatusSummary {
  return {
    id: s.name,
    name: s.name,
    desc: s.description ?? "",
    tools: s.toolCount ?? 0,
    status: s.status,
    errorDetail: s.error ? (s.error.detail ?? s.error.type) : undefined,
    icon: MCP_ICON[s.name] ?? "tool",
  };
}

export function toMcpConfigInfo(c: RpcMCPServerConfig, live?: RpcMCPServer): MCPServerConfigInfo {
  return {
    name: c.name,
    type: c.type,
    enabled: c.enabled,
    description: c.description,
    url: c.url,
    authorizationMasked: c.authorizationMasked,
    command: c.command,
    args: c.args,
    env: c.env,
    dir: c.dir,
    disabledTools: c.disabledTools,
    autoApproveTools: c.autoApproveTools,
    status: live?.status,
    toolCount: live?.toolCount,
    errorDetail: live?.error ? (live.error.detail ?? live.error.type) : undefined,
  };
}

export function toWorkspaceProjectSummary(p: RpcProject): WorkspaceProjectSummary {
  return {
    id: p.cwd,
    name: p.name,
    branch: p.branch ?? "",
    sessionCount: p.sessionCount,
    cwdMissing: p.cwdMissing,
  };
}

const FILE_CHANGE: Record<RpcFileChange["status"], WorkspaceFileChangeSummary["change"]> = {
  added: "add",
  untracked: "add",
  modified: "mod",
  renamed: "mod",
  deleted: "del",
};

export function toWorkspaceFileChangeSummary(f: RpcFileChange): WorkspaceFileChangeSummary {
  return {
    path: f.path,
    change: FILE_CHANGE[f.status],
    added: f.added,
    removed: f.removed,
    binary: f.binary,
  };
}

// Capability-gated workspace reads should render as empty optional surfaces,
// not as broken panes, when the runtime negotiated the feature off.
export function emptyPageIfUngated(err: unknown): { data: never[] } {
  if (isErrorType(err, "capability_not_negotiated")) return { data: [] };
  throw err;
}
