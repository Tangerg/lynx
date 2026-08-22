import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { ServerCapabilities, WireFeature } from "@/rpc";

export interface RuntimeCapabilityPort {
  useCapability(capability: WireFeature): boolean;
  hasCapability(capability: WireFeature): boolean;
  supportsStreamingMethod(method: string): boolean;
  supportsRuntimeTopic(topic: string): boolean;
  /** What the server advertised, or null before discovery. */
  negotiated(): ServerCapabilities | null;
}

const port = createSingletonPort<RuntimeCapabilityPort>(
  "Runtime capability port is not configured",
);

export const configureRuntimeCapabilityPort = port.configure;
export const runtimeCapabilities = port.get;

/**
 * The negotiated capabilities for a caller outside the plugin lifecycle — the SDK's
 * capability preflight, wired at the composition root. Before the adapter installs,
 * null is the true answer rather than an error: nothing has been negotiated.
 */
export function negotiatedCapabilities(): ServerCapabilities | null {
  return port.peek()?.negotiated() ?? null;
}
