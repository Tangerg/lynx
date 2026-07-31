import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { getContainer, resetContainer } from "@/main/container";
import { getConfig, hasConfig, setConfig, useConfigStore } from "@/plugins/sdk/config";
import type { Host } from "@/plugins/sdk";
import {
  applyRuntimeEndpoint,
  currentRuntimeEndpoint,
  DEFAULT_RUNTIME_ENDPOINT,
  resetRuntimeEndpoint,
} from "./runtimeEndpoint";
import { installRuntimeEndpointConfiguration } from "../adapters/runtimeEndpointConfiguration";

const cleanups: Array<() => void> = [];

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
  useConfigStore.setState({ values: new Map(), subscribers: new Map() });
});

afterEach(() => {
  for (const cleanup of cleanups.splice(0).reverse()) cleanup();
  resetContainer();
});

function installConnection(initial?: string) {
  const connection = connectionHost(initial);
  cleanups.push(installRuntimeEndpointConfiguration(connection.host));
  return connection;
}

describe("runtime endpoint", () => {
  it("has a defined read value before the plugin installs its adapter", () => {
    expect(currentRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
    expect(() => applyRuntimeEndpoint("http://127.0.0.1:27171")).toThrow(
      "Runtime endpoint configuration is not installed",
    );
  });

  it("restores the persisted endpoint before Runtime discovery starts", () => {
    installConnection("http://127.0.0.1:27171");

    expect(currentRuntimeEndpoint()).toBe("http://127.0.0.1:27171");
  });

  it("validates, normalizes, and publishes a changed endpoint", () => {
    installConnection();

    const result = applyRuntimeEndpoint("  http://127.0.0.1:27171  ");

    expect(result).toEqual({
      kind: "applied",
      endpoint: "http://127.0.0.1:27171",
      changed: true,
    });
    expect(currentRuntimeEndpoint()).toBe("http://127.0.0.1:27171");
  });

  it("rejects invalid input without changing the active endpoint", () => {
    installConnection();

    const result = applyRuntimeEndpoint("file:///tmp/runtime.sock");

    expect(result).toEqual({
      kind: "rejected",
      input: "file:///tmp/runtime.sock",
      reason: "unsupported_scheme",
    });
    expect(currentRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
  });

  it("distinguishes malformed URLs from unsupported schemes", () => {
    installConnection();

    expect(applyRuntimeEndpoint("not a URL")).toEqual({
      kind: "rejected",
      input: "not a URL",
      reason: "invalid_url",
    });
  });

  it("persists published changes through the Runtime-owned adapter", () => {
    const { stored } = installConnection();

    applyRuntimeEndpoint("http://127.0.0.1:27171");

    expect(stored.get("endpoint")).toBe("http://127.0.0.1:27171");
  });

  it("resets to the default endpoint with honest change metadata", () => {
    installConnection();
    applyRuntimeEndpoint("http://127.0.0.1:27171");

    expect(resetRuntimeEndpoint()).toEqual({
      kind: "applied",
      endpoint: DEFAULT_RUNTIME_ENDPOINT,
      changed: true,
    });
  });

  it("rebuilds the shared Runtime client after an endpoint change", () => {
    installConnection();
    const first = getContainer().client();

    applyRuntimeEndpoint("http://127.0.0.1:27171");
    const second = getContainer().client();

    expect(second).not.toBe(first);
    expect(getContainer().client()).toBe(second);
  });
});
