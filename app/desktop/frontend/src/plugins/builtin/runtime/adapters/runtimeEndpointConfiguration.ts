import type { Host } from "@/plugins/sdk";
import { configureRuntimeEndpoint } from "../application/ports/runtimeEndpoint";

const CONFIG_KEY = "runtime.endpoint";
const STORAGE_KEY = "endpoint";

/**
 * Bind the Runtime endpoint application port to Host configuration and mirror
 * accepted changes into this plugin's persistent storage.
 */
export function installRuntimeEndpointConfiguration(
  host: Pick<Host, "config" | "storage">,
): () => void {
  const stored = host.storage.get<string>(STORAGE_KEY);
  if (typeof stored === "string" && stored.trim()) {
    host.config.set(CONFIG_KEY, stored);
  }

  const disposePort = configureRuntimeEndpoint({
    read: () => host.config.get<string>(CONFIG_KEY),
    write: (endpoint) => host.config.set(CONFIG_KEY, endpoint),
  });

  host.config.onChange(CONFIG_KEY, (value) => {
    if (typeof value === "string") host.storage.set(STORAGE_KEY, value);
  });

  return disposePort;
}
