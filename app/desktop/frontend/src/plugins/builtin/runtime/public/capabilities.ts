import { runtimeCapabilities } from "../application/ports/capabilities";
import type { RuntimeCapability } from "../domain/capability";

export type { RuntimeCapability } from "../domain/capability";

export function useRuntimeCapability(capability: RuntimeCapability): boolean {
  return runtimeCapabilities().useCapability(capability);
}

export function runtimeCapability(capability: RuntimeCapability): boolean {
  return runtimeCapabilities().hasCapability(capability);
}

export function runtimeSupportsStreamingMethod(method: string): boolean {
  return runtimeCapabilities().supportsStreamingMethod(method);
}

export function subscribeRuntimeCapabilities(onChange: () => void): () => void {
  return runtimeCapabilities().subscribe(onChange);
}
