import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { ServerCapabilities } from "@/rpc";
import type { RuntimeCapability } from "../../domain/capability";

export interface RuntimeCapabilityPort {
  useCapability(capability: RuntimeCapability): boolean;
  hasCapability(capability: RuntimeCapability): boolean;
  supportsStreamingMethod(method: string): boolean;
  subscribe(onChange: () => void): () => void;
  replace(capabilities: ServerCapabilities): void;
  clear(): void;
}

const port = createSingletonPort<RuntimeCapabilityPort>(
  "Runtime capability port is not configured",
);

export const configureRuntimeCapabilityPort = port.configure;
export const runtimeCapabilities = port.get;
