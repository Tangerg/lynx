import { getContainer } from "@/main/container";
import { errorDetail, type ConfigureMCPServerRequest } from "@/rpc";
import { t } from "@/lib/i18n";
import { configureMCPServerGateway } from "../application/ports/mcpServerGateway";
import type { MCPServerGateway } from "../application/ports/mcpServerGateway";
import type { MCPServerConfigInput } from "../application/mcpServerInput";

function configureRequest(input: MCPServerConfigInput): ConfigureMCPServerRequest {
  const base = {
    name: input.name,
    type: input.transport,
    enabled: input.enabled,
    description: input.description,
    timeoutSeconds: input.timeoutSeconds,
    disabledTools: input.disabledTools,
    autoApproveTools: input.autoApproveTools,
  } satisfies ConfigureMCPServerRequest;

  if (input.transport === "stdio") {
    return {
      ...base,
      command: input.command,
      args: input.args,
      env: input.env,
      dir: input.dir,
    };
  }

  return {
    ...base,
    url: input.url,
    authorization: input.authorization,
    headers: input.headers,
  };
}

const gateway: MCPServerGateway = {
  async configure(input) {
    await getContainer().client().workspace.mcp.configure(configureRequest(input));
  },
  async remove(name) {
    await getContainer().client().workspace.mcp.remove(name);
  },
  async setEnabled(name, enabled) {
    await getContainer().client().workspace.mcp.setEnabled(name, enabled);
  },
  async authorize(name) {
    await getContainer().client().workspace.mcp.authorize(name);
  },
  async test(input) {
    const result = await getContainer().client().workspace.mcp.test(configureRequest(input));
    return {
      ok: result.ok,
      error: result.ok ? undefined : (errorDetail(result.error) ?? t("mcp.error.test")),
    };
  },
};

export function installMCPServerGateway(): void {
  configureMCPServerGateway(gateway);
}
