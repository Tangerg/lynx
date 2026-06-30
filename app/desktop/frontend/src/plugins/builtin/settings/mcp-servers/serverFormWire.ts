import type { MCPServerConfigInfo, MCPTransport } from "@/lib/data/queries";
import type { ConfigureMCPServerRequest } from "@/rpc";

export interface ServerFormDraft {
  name: string;
  transport: MCPTransport;
  description: string;
  command: string;
  args: string;
  env: string;
  dir: string;
  url: string;
  authorization: string;
  headers: string;
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

function mapToLines(map: Record<string, string> | undefined): string {
  return map
    ? Object.entries(map)
        .map(([key, value]) => `${key}=${value}`)
        .join("\n")
    : "";
}

export function initialServerFormDraft(server?: MCPServerConfigInfo): ServerFormDraft {
  return {
    name: server?.name ?? "",
    transport: server?.type ?? "stdio",
    description: server?.description ?? "",
    command: server?.command ?? "",
    args: (server?.args ?? []).join("\n"),
    env: mapToLines(server?.env),
    dir: server?.dir ?? "",
    url: server?.url ?? "",
    authorization: "",
    headers: mapToLines(server?.headers),
    timeoutSec: server?.timeoutSeconds ? String(server.timeoutSeconds) : "",
    disabledTools: server?.disabledTools ?? [],
    autoApproveTools: server?.autoApproveTools ?? [],
  };
}

export function isServerFormDraftValid(draft: ServerFormDraft): boolean {
  return (
    draft.name.trim() !== "" &&
    (draft.transport === "stdio" ? draft.command.trim() !== "" : draft.url.trim() !== "")
  );
}

export function serverFormRequest(
  draft: ServerFormDraft,
  server?: MCPServerConfigInfo,
): ConfigureMCPServerRequest {
  const secs = parseInt(draft.timeoutSec, 10);
  const base: ConfigureMCPServerRequest = {
    name: draft.name.trim(),
    type: draft.transport,
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
      env: linesToMap(draft.env),
      dir: draft.dir.trim() || undefined,
    };
  }
  return {
    ...base,
    url: draft.url.trim() || undefined,
    authorization: draft.authorization.trim() || undefined,
    headers: linesToMap(draft.headers),
  };
}
