import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface InvokeDiagnosticToolInput {
  name: string;
  arguments: Record<string, unknown>;
  cwd?: string;
}

export interface ToolCatalogGateway {
  reconnectMCPServer(server: string): Promise<void>;
  invokeDiagnosticTool(input: InvokeDiagnosticToolInput): Promise<unknown>;
}

const port = createSingletonPort<ToolCatalogGateway>("Tool catalog gateway is not configured");

export const configureToolCatalogGateway = port.configure;
export const toolCatalogGateway = port.get;
