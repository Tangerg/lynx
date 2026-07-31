import { createSingletonPort } from "@/lib/ports/singletonPort";

/**
 * The application-facing source of the active Runtime endpoint.
 *
 * The port deliberately says nothing about Zustand, Host configuration or
 * persistence. Those are adapter mechanisms; the use case only needs to read
 * and replace one value.
 */
export interface RuntimeEndpointConfiguration {
  read(): string | undefined;
  write(endpoint: string): void;
}

const port = createSingletonPort<RuntimeEndpointConfiguration>(
  "Runtime endpoint configuration is not installed",
);

export const configureRuntimeEndpoint = port.configure;
export const runtimeEndpointConfiguration = port.get;
export const configuredRuntimeEndpoint = port.peek;
