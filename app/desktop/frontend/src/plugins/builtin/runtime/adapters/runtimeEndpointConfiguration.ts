import type { ConfigService, KeyValueStore } from "@/plugins/sdk";
import { configureRuntimeEndpoint } from "../application/ports/runtimeEndpoint";

const CONFIG_KEY = "runtime.endpoint";
const STORAGE_KEY = "endpoint";

/**
 * Bind the Runtime endpoint application port to Host configuration and mirror
 * accepted changes into this plugin's persistent storage.
 */
interface EndpointBindings {
  config: ConfigService;
  storage: KeyValueStore;
}

export type ReplaceRuntimeEndpoint = (commit: () => void) => void;

export function installRuntimeEndpointConfiguration(
  ctx: EndpointBindings,
  replaceConnection: ReplaceRuntimeEndpoint,
): () => void {
  const stored = ctx.storage.get(STORAGE_KEY);
  if (typeof stored === "string" && stored.trim()) {
    ctx.config.set(CONFIG_KEY, stored);
  }

  const disposePort = configureRuntimeEndpoint({
    read: () => {
      const value = ctx.config.get(CONFIG_KEY);
      return typeof value === "string" ? value : undefined;
    },
    replace: (endpoint) => replaceConnection(() => ctx.config.set(CONFIG_KEY, endpoint)),
  });

  const disposeStorageMirror = ctx.config.onChange(CONFIG_KEY, (value) => {
    if (typeof value === "string") ctx.storage.set(STORAGE_KEY, value);
  });

  return () => {
    disposeStorageMirror.dispose();
    disposePort();
  };
}
