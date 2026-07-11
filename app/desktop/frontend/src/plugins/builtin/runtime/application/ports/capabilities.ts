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

let port: RuntimeCapabilityPort | null = null;

export function configureRuntimeCapabilityPort(next: RuntimeCapabilityPort): void {
  port = next;
}

export function runtimeCapabilities(): RuntimeCapabilityPort {
  if (!port) throw new Error("Runtime capability port is not configured");
  return port;
}
