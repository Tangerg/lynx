import { DESKTOP_CLIENT_INFO } from "@/main/config";
import { getConfig } from "@/plugins/sdk/config";
import type { ObservabilityTeardown } from "./observabilityLifecycle";

export async function initFrontendObservability(): Promise<ObservabilityTeardown> {
  const { setupObservability, teardownObservability } = await import("@/lib/observability/setup");
  const configuredEndpoint = getConfig("otel.endpoint");
  await setupObservability({
    serviceName: "lyra-frontend",
    serviceVersion: DESKTOP_CLIENT_INFO.version,
    otlpEndpoint: typeof configuredEndpoint === "string" ? configuredEndpoint : undefined,
  });
  return teardownObservability;
}
