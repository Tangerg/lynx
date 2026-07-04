import { CLIENT_INFO } from "@/main/config";
import { getConfig } from "@/plugins/sdk/config";
import type { BootstrapTeardown } from "../application/bootstrapLifecycle";

export async function initFrontendObservability(): Promise<BootstrapTeardown> {
  const { setupObservability, teardownObservability } = await import("@/lib/observability/setup");
  await setupObservability({
    serviceName: "lyra-frontend",
    serviceVersion: CLIENT_INFO.version,
    otlpEndpoint: getConfig<string>("otel.endpoint") ?? undefined,
  });
  return teardownObservability;
}
