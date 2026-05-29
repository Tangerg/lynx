import { afterEach, describe, expect, it } from "vitest";
import type { Methods } from "@/rpc";
import { getContainer, resetContainer, setContainer } from "./container";

describe("main/container", () => {
  afterEach(resetContainer);

  it("exposes the Runtime Protocol entry points out of the box", () => {
    const c = getContainer();
    expect(typeof c.createRpc).toBe("function");
    expect(typeof c.methods).toBe("function");
    expect(c.sidecar).toBeDefined();
  });

  it("setContainer() swaps a single slot, leaving others intact", () => {
    const fakeMethods = {} as Methods;
    const before = getContainer().sidecar;
    setContainer({ methods: () => fakeMethods });
    expect(getContainer().methods()).toBe(fakeMethods);
    expect(getContainer().sidecar).toBe(before);
  });

  it("resetContainer() restores defaults", () => {
    const fakeMethods = {} as Methods;
    setContainer({ methods: () => fakeMethods });
    resetContainer();
    expect(getContainer().methods()).not.toBe(fakeMethods);
  });

  it("methods() returns a cached singleton (one client for the container's life)", () => {
    const first = getContainer().methods();
    expect(getContainer().methods()).toBe(first);
    resetContainer();
    expect(getContainer().methods()).not.toBe(first);
  });
});
