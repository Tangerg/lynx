import type { MCPServerSettings, MCPTransport } from "./mcpServerConfig";
import type { MCPServerInput } from "./mcpServerInput";

export interface MCPServerDraft {
  name: string;
  transport: MCPTransport;
  description: string;
  command: string;
  args: string;
  env: string;
  clearEnvironment: boolean;
  dir: string;
  url: string;
  authorization: string;
  clearAuthorization: boolean;
  headers: string;
  clearHeaders: boolean;
  timeoutSec: string;
  disabledTools: string[];
  autoApproveTools: string[];
}

function linesToList(text: string): string[] | undefined {
  const list = text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  return list.length ? list : undefined;
}

function linesToMap(text: string): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    const i = line.indexOf("=");
    if (i === -1) out[line] = "";
    else out[line.slice(0, i)] = line.slice(i + 1);
  }
  return Object.keys(out).length ? out : undefined;
}

export function initialMCPServerDraft(server?: MCPServerSettings): MCPServerDraft {
  return {
    name: server?.name ?? "",
    transport: server?.type ?? "stdio",
    description: server?.description ?? "",
    command: server?.command ?? "",
    args: (server?.args ?? []).join("\n"),
    env: "",
    clearEnvironment: false,
    dir: server?.dir ?? "",
    url: server?.url ?? "",
    authorization: "",
    clearAuthorization: false,
    headers: "",
    clearHeaders: false,
    timeoutSec: server?.timeoutSeconds ? String(server.timeoutSeconds) : "",
    disabledTools: server?.disabledTools ?? [],
    autoApproveTools: server?.autoApproveTools ?? [],
  };
}

export function isMCPServerDraftValid(draft: MCPServerDraft, server?: MCPServerSettings): boolean {
  return (
    draft.name.trim() !== "" &&
    (draft.transport === "stdio" ? draft.command.trim() !== "" : draft.url.trim() !== "") &&
    !mcpAuthorizationNeedsDisposition(draft, server) &&
    !mcpHeadersNeedDisposition(draft, server) &&
    !mcpEnvironmentNeedsDisposition(draft, server)
  );
}

export function mcpEnvironmentNeedsDisposition(
  draft: MCPServerDraft,
  server?: MCPServerSettings,
): boolean {
  return (
    draft.transport === "stdio" &&
    linesToMap(draft.env) === undefined &&
    !draft.clearEnvironment &&
    server?.type === "stdio" &&
    Boolean(server.envMasked && Object.keys(server.envMasked).length > 0) &&
    !sameStdioTarget(server, draft)
  );
}

export function mcpHeadersNeedDisposition(
  draft: MCPServerDraft,
  server?: MCPServerSettings,
): boolean {
  return (
    draft.transport === "streamableHttp" &&
    linesToMap(draft.headers) === undefined &&
    !draft.clearHeaders &&
    server?.type === "streamableHttp" &&
    Boolean(server.headersMasked && Object.keys(server.headersMasked).length > 0) &&
    !sameHTTPOrigin(server.url, draft.url)
  );
}

export function mcpAuthorizationNeedsDisposition(
  draft: MCPServerDraft,
  server?: MCPServerSettings,
): boolean {
  return (
    draft.transport === "streamableHttp" &&
    draft.authorization.trim() === "" &&
    !draft.clearAuthorization &&
    server?.type === "streamableHttp" &&
    Boolean(server.authorizationMasked) &&
    !sameHTTPOrigin(server.url, draft.url)
  );
}

export function mcpServerInputFromDraft(
  draft: MCPServerDraft,
  server?: MCPServerSettings,
): MCPServerInput {
  const secs = parseInt(draft.timeoutSec, 10);
  const base: MCPServerInput = {
    name: draft.name.trim(),
    transport: draft.transport,
    enabled: server?.enabled ?? true,
    description: draft.description.trim() || undefined,
    timeoutSeconds: Number.isFinite(secs) && secs > 0 ? secs : undefined,
    disabledTools: draft.disabledTools.length ? draft.disabledTools : undefined,
    autoApproveTools: draft.autoApproveTools.length ? draft.autoApproveTools : undefined,
  };
  if (draft.transport === "stdio") {
    return {
      ...base,
      command: draft.command.trim() || undefined,
      args: linesToList(draft.args),
      env: environmentFromDraft(draft),
      dir: draft.dir.trim() || undefined,
    };
  }
  return {
    ...base,
    url: draft.url.trim() || undefined,
    authorization: authorizationFromDraft(draft),
    headers: headersFromDraft(draft),
  };
}

function authorizationFromDraft(draft: MCPServerDraft): string | null | undefined {
  const entered = draft.authorization.trim();
  if (entered) return entered;
  if (draft.clearAuthorization) return null;
  return undefined;
}

function headersFromDraft(draft: MCPServerDraft): Record<string, string> | null | undefined {
  const headers = linesToMap(draft.headers);
  if (headers) return headers;
  return draft.clearHeaders ? null : undefined;
}

function environmentFromDraft(draft: MCPServerDraft): Record<string, string> | null | undefined {
  const environment = linesToMap(draft.env);
  if (environment) return environment;
  return draft.clearEnvironment ? null : undefined;
}

function sameStdioTarget(server: MCPServerSettings, draft: MCPServerDraft): boolean {
  const args = linesToList(draft.args) ?? [];
  const storedArgs = server.args ?? [];
  return (
    server.command === draft.command.trim() &&
    storedArgs.length === args.length &&
    storedArgs.every((value, index) => value === args[index]) &&
    (server.dir ?? "") === draft.dir.trim()
  );
}

function sameHTTPOrigin(left: string | undefined, right: string): boolean {
  try {
    return new URL(left ?? "").origin === new URL(right).origin;
  } catch {
    return false;
  }
}
