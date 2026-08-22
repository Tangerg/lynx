import { runtimeCapabilities, type RuntimeCapabilityPort } from "../application/ports/capabilities";

export { negotiatedCapabilities } from "../application/ports/capabilities";

// These borrow their signatures FROM the port rather than restating them. The
// capability keys are the runtime's published vocabulary, which left two honest
// alternatives — name the wire type here, which a public surface may not do, or mint
// a second word for it, which would give one concept two identities. Taking the
// port's signature does neither: the context speaks its own contract, and the
// vocabulary keeps one owner.

export const useRuntimeCapability: RuntimeCapabilityPort["useCapability"] = (capability) =>
  runtimeCapabilities().useCapability(capability);

export const runtimeCapability: RuntimeCapabilityPort["hasCapability"] = (capability) =>
  runtimeCapabilities().hasCapability(capability);

export function runtimeSupportsStreamingMethod(method: string): boolean {
  return runtimeCapabilities().supportsStreamingMethod(method);
}

export const runtimeSupportsTopic: RuntimeCapabilityPort["supportsRuntimeTopic"] = (topic) =>
  runtimeCapabilities().supportsRuntimeTopic(topic);
