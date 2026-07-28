import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { ServerCapabilities, WireFeature } from "@/rpc";

export interface RuntimeCapabilityPort {
  useCapability(capability: WireFeature): boolean;
  hasCapability(capability: WireFeature): boolean;
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
