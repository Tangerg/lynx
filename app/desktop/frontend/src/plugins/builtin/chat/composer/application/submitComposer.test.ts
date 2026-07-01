import { describe, expect, it, vi } from "vitest";
import { definePlugin, loadPlugin } from "@/plugins/sdk";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import { submitComposer, type SubmitDeps } from "./submitComposer";

describe("submitComposer", () => {
  function deps(input: Partial<SubmitDeps>): SubmitDeps {
    return {
      value: "",
      clear: () => {},
      sendInput: () => {},
      images: [],
      pastes: [],
      recordHistory: () => {},
      ...input,
    };
  }

  it("is a no-op on empty / whitespace-only input", () => {
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer(deps({ value: "   ", clear, sendInput: send }));
    expect(send).not.toHaveBeenCalled();
    expect(clear).not.toHaveBeenCalled();
  });

  it("forwards plain text as user input then clears", () => {
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer(deps({ value: "hello", clear, sendInput: send }));
    expect(send).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "hello" }] });
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
    submitComposer(deps({ value: "/echo hi there", clear, sendInput: send }));
    // The slash handler gets a text→input adapter, not sendInput itself.
    expect(run).toHaveBeenCalledWith({ args: "hi there", send: expect.any(Function) });
    expect(send).not.toHaveBeenCalled();
    expect(clear).toHaveBeenCalledOnce();
  });

  it("falls back to sendInput for an unknown slash command", () => {
    const send = vi.fn();
    submitComposer(deps({ value: "/unknown args", sendInput: send }));
    expect(send).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "/unknown args" }] });
  });

  it("folds pasted-text attachments into the message below the typed text", () => {
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer(
      deps({
        value: "look at this",
        clear,
        sendInput: send,
        pastes: [{ id: "p1", text: "PASTED BLOB", lines: 1 }],
      }),
    );
    expect(send).toHaveBeenCalledWith({
      parts: [{ kind: "text", text: "look at this\n\nPASTED BLOB" }],
    });
    expect(clear).toHaveBeenCalledOnce();
  });

  it("allows a paste-only send (no typed text)", () => {
    const send = vi.fn();
    submitComposer(
      deps({
        value: "   ",
        sendInput: send,
        pastes: [{ id: "p1", text: "ONLY PASTE", lines: 1 }],
      }),
    );
    expect(send).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "ONLY PASTE" }] });
  });
});
