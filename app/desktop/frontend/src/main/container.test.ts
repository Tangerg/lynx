import { afterEach, describe, expect, it } from "vitest";
import { RUNTIME_BASE, RUNTIME_ENDPOINT_CONFIG_KEY } from "@/main/config";
import { setConfig } from "@/plugins/sdk/config";
import type { LyraClient } from "@/rpc";
import { getContainer, resetContainer, setContainer } from "./container";

describe("main/container", () => {
  afterEach(() => {
    setConfig(RUNTIME_ENDPOINT_CONFIG_KEY, RUNTIME_BASE);
    resetContainer();
  });

  it("exposes the Runtime Protocol entry points out of the box", () => {
    const c = getContainer();
    expect(typeof c.client).toBe("function");
    expect(c.shell).toBeDefined();
  });

  it("setContainer() swaps a single slot, leaving others intact", () => {
    const fake = {} as LyraClient;
    const before = getContainer().shell;
    setContainer({ client: () => fake });
    expect(getContainer().client()).toBe(fake);
    expect(getContainer().shell).toBe(before);
  });

  it("resetContainer() restores defaults", () => {
    const fake = {} as LyraClient;
    setContainer({ client: () => fake });
    resetContainer();
    expect(getContainer().client()).not.toBe(fake);
  });

  it("client() returns a cached singleton (one SDK client for the container's life)", () => {
    const first = getContainer().client();
    expect(getContainer().client()).toBe(first);
    resetContainer();
    expect(getContainer().client()).not.toBe(first);
  });

  it("rebuilds the Runtime client when the configured endpoint changes", () => {
    const first = getContainer().client();
    setConfig(RUNTIME_ENDPOINT_CONFIG_KEY, "http://127.0.0.1:27171");

    const second = getContainer().client();

    expect(second).not.toBe(first);
    expect(getContainer().client()).toBe(second);
  });
});
