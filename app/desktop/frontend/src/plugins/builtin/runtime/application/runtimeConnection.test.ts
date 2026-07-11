import { beforeEach, describe, expect, it } from "vitest";
import { RUNTIME_BASE, RUNTIME_ENDPOINT_CONFIG_KEY } from "@/main/config";
import {
  getConfig,
  hasConfig,
  setConfig,
  useConfigStore,
  type ConfigValue,
} from "@/plugins/sdk/config";
import type { Host } from "@/plugins/sdk";
import {
  applyRuntimeEndpoint,
  currentRuntimeEndpoint,
  installRuntimeConnection,
  resetRuntimeEndpoint,
} from "./runtimeConnection";

function connectionHost(initial?: string): {
  host: Pick<Host, "config" | "storage">;
  stored: Map<string, unknown>;
} {
  const stored = new Map<string, unknown>();
  if (initial) stored.set("endpoint", initial);
  return {
    stored,
    host: {
      config: {
        get: getConfig,
        set: setConfig,
        has: hasConfig,
        onChange: (key, fn) => useConfigStore.getState().subscribe(key, fn),
      },
      storage: {
        get: <T>(key: string) => stored.get(key) as T | undefined,
        set: <T>(key: string, value: T) => {
          stored.set(key, value);
        },
        remove: (key) => {
          stored.delete(key);
        },
        keys: () => [...stored.keys()],
      },
    },
  };
}

beforeEach(() => {
  setConfig(RUNTIME_ENDPOINT_CONFIG_KEY, RUNTIME_BASE);
});

describe("runtime connection", () => {
  it("restores the persisted endpoint before Runtime discovery starts", () => {
    const { host } = connectionHost("http://127.0.0.1:27171");

    installRuntimeConnection(host);

    expect(currentRuntimeEndpoint()).toBe("http://127.0.0.1:27171");
  });

  it("validates, normalizes, and publishes a changed endpoint", () => {
    const result = applyRuntimeEndpoint("  http://127.0.0.1:27171  ");

    expect(result).toEqual({
      endpoint: "http://127.0.0.1:27171",
      error: null,
      changed: true,
    });
    expect(getConfig<string>(RUNTIME_ENDPOINT_CONFIG_KEY)).toBe("http://127.0.0.1:27171");
  });

  it("rejects invalid input without changing the active endpoint", () => {
    const result = applyRuntimeEndpoint("file:///tmp/runtime.sock");

    expect(result.error).not.toBeNull();
    expect(result.changed).toBe(false);
    expect(currentRuntimeEndpoint()).toBe(RUNTIME_BASE);
  });

  it("persists published changes through the Runtime-owned adapter", () => {
    const { host, stored } = connectionHost();
    installRuntimeConnection(host);

    setConfig(RUNTIME_ENDPOINT_CONFIG_KEY, "http://127.0.0.1:27171" as ConfigValue);

    expect(stored.get("endpoint")).toBe("http://127.0.0.1:27171");
  });

  it("resets to the default endpoint with honest change metadata", () => {
    setConfig(RUNTIME_ENDPOINT_CONFIG_KEY, "http://127.0.0.1:27171");

    expect(resetRuntimeEndpoint()).toEqual({
      endpoint: RUNTIME_BASE,
      error: null,
      changed: true,
    });
  });
});
