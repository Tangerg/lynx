import { serverFeature, useServerFeature, type ServerFeature } from "@/state/runtimeStore";

export type RuntimeCapability = ServerFeature;

export function useRuntimeCapability(capability: RuntimeCapability): boolean {
  return useServerFeature(capability);
}

export function runtimeCapability(capability: RuntimeCapability): boolean {
  return serverFeature(capability);
}
