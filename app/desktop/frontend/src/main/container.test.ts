import { afterEach, describe, expect, it } from "vitest";
import type { LyraClient } from "@/rpc";
import { getContainer, resetContainer, setContainer } from "./container";

describe("main/container", () => {
  afterEach(resetContainer);

  it("exposes the Runtime Protocol entry points out of the box", () => {
    const c = getContainer();
    expect(typeof c.client).toBe("function");
    expect(c.sidecar).toBeDefined();
  });

  it("setContainer() swaps a single slot, leaving others intact", () => {
    const fake = {} as LyraClient;
    const before = getContainer().sidecar;
    setContainer({ client: () => fake });
    expect(getContainer().client()).toBe(fake);
    expect(getContainer().sidecar).toBe(before);
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
});
