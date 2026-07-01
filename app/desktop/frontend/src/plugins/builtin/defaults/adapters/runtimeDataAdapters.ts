import type {
  FileChange as SidebarFileChange,
  MCPServer as SidebarMCPServer,
  MCPServerConfigInfo,
  SidebarProject,
  SidebarSession,
} from "@/lib/data/queries";
import type {
  McpServer as RpcMCPServer,
  McpServerConfig as RpcMCPServerConfig,
  Project as RpcProject,
  Session,
  WorkspaceFileChange as RpcFileChange,
} from "@/rpc";
import { isErrorType } from "@/rpc";

export function toSidebarSession(s: Session): SidebarSession {
  return {
    id: s.id,
    title: s.title,
    status: s.status,
    model: s.model,
    cwd: s.cwd,
    cwdMissing: s.cwdMissing,
    usage: s.usage
      ? {
          inputTokens: s.usage.inputTokens,
          outputTokens: s.usage.outputTokens,
          costUsd: s.usage.costUsd,
        }
      : undefined,
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

export function toSidebarMCPServer(s: RpcMCPServer): SidebarMCPServer {
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

export function toSidebarProject(p: RpcProject): SidebarProject {
  return {
    id: p.cwd,
    name: p.name,
    branch: p.branch ?? "",
    sessionCount: p.sessionCount,
    cwdMissing: p.cwdMissing,
  };
}

const FILE_CHANGE: Record<RpcFileChange["status"], SidebarFileChange["change"]> = {
  added: "add",
  untracked: "add",
  modified: "mod",
  renamed: "mod",
  deleted: "del",
};

export function toSidebarFileChange(f: RpcFileChange): SidebarFileChange {
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
