import { describe, expect, it, vi } from "vitest";
import { definePlugin, loadPlugin } from "@/plugins/sdk";
import { parseSlash, submitComposer } from "./submitComposer";

describe("parseSlash", () => {
  it("splits on the first whitespace", () => {
    expect(parseSlash("/lint src/foo.ts")).toEqual({ cmd: "/lint", args: "src/foo.ts" });
  });

  it("returns empty args when there's no whitespace", () => {
    expect(parseSlash("/diff")).toEqual({ cmd: "/diff", args: "" });
  });

  it("returns null for plain text without a leading slash", () => {
    expect(parseSlash("hello there")).toBeNull();
  });
});

describe("submitComposer", () => {
  it("is a no-op on empty / whitespace-only input", () => {
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer({ value: "   ", clear, sendText: send });
    expect(send).not.toHaveBeenCalled();
    expect(clear).not.toHaveBeenCalled();
  });

  it("forwards plain text to sendText then clears", () => {
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer({ value: "hello", clear, sendText: send });
    expect(send).toHaveBeenCalledWith("hello");
    expect(clear).toHaveBeenCalledOnce();
  });

  it("routes a registered slash command to its handler — sendText not called", async () => {
    const run = vi.fn();
    await loadPlugin(
      definePlugin({
        name: "test.submit.slash",
        version: "1.0.0",
        setup: ({ host }) => {
          host.composer.registerCommand("/echo", { description: "echo", run });
        },
      }),
    );
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer({ value: "/echo hi there", clear, sendText: send });
    expect(run).toHaveBeenCalledWith({ args: "hi there", send });
    expect(send).not.toHaveBeenCalled();
    expect(clear).toHaveBeenCalledOnce();
  });

  it("falls back to sendText for an unknown slash command", () => {
    const send = vi.fn();
    submitComposer({ value: "/unknown args", clear: () => {}, sendText: send });
    expect(send).toHaveBeenCalledWith("/unknown args");
  });
});
