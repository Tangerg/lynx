import { t } from "@/lib/i18n";
import { describeProblem } from "@/lib/rpcErrors";
import { getContainer } from "@/main/container";
import type {
  MCPServerCandidate,
  MCPAuthorizationChange,
  MCPAuthorizationAttempt,
  MCPConnectionInput,
  MCPEnvironmentChange,
  MCPHeadersChange,
  UpdateMCPServerRequest,
} from "@/rpc";
import type { MCPServerInput } from "../application/mcpServerInput";
import { mcpServerSettings } from "./runtimeMcpServerProjection";
import {
  configureMCPServerGateway,
  type MCPAuthorizationAttempt as AuthorizationAttempt,
  type MCPServerGateway,
} from "../application/ports/mcpServerGateway";

function authorizationChange(value: string | null | undefined): MCPAuthorizationChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

function headersChange(
  value: Record<string, string> | null | undefined,
): MCPHeadersChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

function environmentChange(
  value: Record<string, string> | null | undefined,
): MCPEnvironmentChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

function connectionInput(input: MCPServerInput): MCPConnectionInput {
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

function authorizationAttempt(attempt: MCPAuthorizationAttempt): AuthorizationAttempt {
  switch (attempt.status.type) {
    case "pending":
    case "succeeded":
    case "canceled":
      return { id: attempt.id, status: attempt.status.type };
    case "failed":
      return {
        id: attempt.id,
        status: "failed",
        error: describeProblem(attempt.status.error) ?? t("mcp.error.signIn"),
      };
  }
}

const gateway: MCPServerGateway = {
  async create(input) {
    const saved = await getContainer().client().mcp.create(candidate(input));
    return mcpServerSettings(saved);
  },
  async update(name, input) {
    const saved = await getContainer().client().mcp.update(updateRequest(name, input));
    return mcpServerSettings(saved);
  },
  async delete(name) {
    await getContainer().client().mcp.delete(name);
  },
  async setEnabled(name, enabled) {
    const saved = await getContainer().client().mcp.update({ server: name, enabled });
    return mcpServerSettings(saved);
  },
  async createAuthorizationAttempt(name, signal) {
    const attempt = await getContainer().client().mcp.authorizationAttempts.create(name, signal);
    return authorizationAttempt(attempt);
  },
  async getAuthorizationAttempt(id, signal) {
    const attempt = await getContainer().client().mcp.authorizationAttempts.get(id, signal);
    return authorizationAttempt(attempt);
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
