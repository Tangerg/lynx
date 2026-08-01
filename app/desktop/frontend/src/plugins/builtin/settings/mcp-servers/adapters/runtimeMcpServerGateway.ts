import { t } from "@/lib/i18n";
import { describeProblem } from "@/lib/rpcErrors";
import { getContainer } from "@/main/container";
import type {
  MCPServerCandidate,
  McpAuthorizationChange,
  McpConnectionInput,
  McpEnvironmentChange,
  McpHeadersChange,
  UpdateMCPServerRequest,
} from "@/rpc";
import type { MCPServerInput } from "../application/mcpServerInput";
import {
  configureMCPServerGateway,
  type MCPServerGateway,
} from "../application/ports/mcpServerGateway";

function authorizationChange(value: string | null | undefined): McpAuthorizationChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

function headersChange(
  value: Record<string, string> | null | undefined,
): McpHeadersChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

function environmentChange(
  value: Record<string, string> | null | undefined,
): McpEnvironmentChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

function connectionInput(input: MCPServerInput): McpConnectionInput {
  if (input.transport === "stdio") {
    return {
      type: "stdio",
      command: input.command ?? "",
      args: input.args,
      env: environmentChange(input.env),
      dir: input.dir,
    };
  }
  return {
    type: "streamableHttp",
    url: input.url ?? "",
    authorization: authorizationChange(input.authorization),
    headers: headersChange(input.headers),
  };
}

function candidate(input: MCPServerInput): MCPServerCandidate {
  return {
    name: input.name,
    enabled: input.enabled,
    description: input.description,
    connection: connectionInput(input),
    timeoutSeconds: input.timeoutSeconds,
    disabledTools: input.disabledTools,
    autoApproveTools: input.autoApproveTools,
  };
}

function updateRequest(name: string, input: MCPServerInput): UpdateMCPServerRequest {
  return {
    server: name,
    description: input.description ?? "",
    connection: connectionInput(input),
    timeoutSeconds: input.timeoutSeconds ?? 0,
    disabledTools: input.disabledTools ?? [],
    autoApproveTools: input.autoApproveTools ?? [],
  };
}

const gateway: MCPServerGateway = {
  async create(input) {
    await getContainer().client().mcp.create(candidate(input));
  },
  async update(name, input) {
    await getContainer().client().mcp.update(updateRequest(name, input));
  },
  async delete(name) {
    await getContainer().client().mcp.delete(name);
  },
  async setEnabled(name, enabled) {
    await getContainer().client().mcp.update({ server: name, enabled });
  },
  async authorize(name) {
    await getContainer().client().mcp.authorize(name);
  },
  async test(input) {
    const result = await getContainer().client().mcp.test(candidate(input));
    return {
      ok: result.ok,
      error: result.ok ? undefined : (describeProblem(result.error) ?? t("mcp.error.test")),
    };
  },
};

export function installMCPServerGateway(): () => void {
  return configureMCPServerGateway(gateway);
}
