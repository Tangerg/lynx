import { afterEach, describe, expect, it, vi } from "vitest";
import { definePlugin, loadPlugin } from "@/plugins/sdk";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import { useComposerStore } from "../adapters/composerStore";
import { submitComposer } from "./submitComposer";

describe("submitComposer", () => {
  // Pastes are read off the composer store; reset after each so the cases that
  // assume no attachments aren't polluted by a prior test.
  afterEach(() => useComposerStore.setState({ pastes: [] }));

  it("is a no-op on empty / whitespace-only input", () => {
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer({ value: "   ", clear, sendInput: send, images: [] });
    expect(send).not.toHaveBeenCalled();
    expect(clear).not.toHaveBeenCalled();
  });

  it("forwards plain text as user input then clears", () => {
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer({ value: "hello", clear, sendInput: send, images: [] });
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
    submitComposer({ value: "/echo hi there", clear, sendInput: send, images: [] });
    // The slash handler gets a text→input adapter, not sendInput itself.
    expect(run).toHaveBeenCalledWith({ args: "hi there", send: expect.any(Function) });
    expect(send).not.toHaveBeenCalled();
    expect(clear).toHaveBeenCalledOnce();
  });

  it("falls back to sendInput for an unknown slash command", () => {
    const send = vi.fn();
    submitComposer({ value: "/unknown args", clear: () => {}, sendInput: send, images: [] });
    expect(send).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "/unknown args" }] });
  });

  it("folds pasted-text attachments into the message below the typed text", () => {
    useComposerStore.setState({ pastes: [{ id: "p1", text: "PASTED BLOB", lines: 1 }] });
    const send = vi.fn();
    const clear = vi.fn();
    submitComposer({ value: "look at this", clear, sendInput: send, images: [] });
    expect(send).toHaveBeenCalledWith({
      parts: [{ kind: "text", text: "look at this\n\nPASTED BLOB" }],
    });
    expect(clear).toHaveBeenCalledOnce();
  });

  it("allows a paste-only send (no typed text)", () => {
    useComposerStore.setState({ pastes: [{ id: "p1", text: "ONLY PASTE", lines: 1 }] });
    const send = vi.fn();
    submitComposer({ value: "   ", clear: () => {}, sendInput: send, images: [] });
    expect(send).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "ONLY PASTE" }] });
  });
});
