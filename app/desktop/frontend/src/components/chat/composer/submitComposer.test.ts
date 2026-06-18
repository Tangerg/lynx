import { describe, expect, it, vi } from "vitest";
import { definePlugin, loadPlugin } from "@/plugins/sdk";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
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
    submitComposer({ value: "   ", clear, sendInput: send, images: [] });
    expect(send).not.toHaveBeenCalled();
    expect(clear).not.toHaveBeenCalled();
  });

  it("forwards plain text as a text ContentBlock to sendInput then clears", () => {
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer({ value: "hello", clear, sendInput: send, images: [] });
    expect(send).toHaveBeenCalledWith([{ type: "text", text: "hello" }]);
    expect(clear).toHaveBeenCalledOnce();
  });

  it("routes a registered slash command to its handler — sendInput not called", async () => {
    const run = vi.fn();
    await loadPlugin(
      definePlugin({
        name: "test.submit.slash",
        version: "1.0.0",
        setup: ({ host }) => {
          host.extensions.contribute(SLASH_COMMAND, { description: "echo", run }, { key: "/echo" });
        },
      }),
    );
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer({ value: "/echo hi there", clear, sendInput: send, images: [] });
    // The slash handler gets a text→input adapter, not sendInput itself.
    expect(run).toHaveBeenCalledWith({ args: "hi there", send: expect.any(Function) });
    expect(send).not.toHaveBeenCalled();
    expect(clear).toHaveBeenCalledOnce();
  });

  it("falls back to sendInput for an unknown slash command", () => {
    const send = vi.fn();
    submitComposer({ value: "/unknown args", clear: () => {}, sendInput: send, images: [] });
    expect(send).toHaveBeenCalledWith([{ type: "text", text: "/unknown args" }]);
  });
});
