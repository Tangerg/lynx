import { afterEach, describe, expect, it, vi } from "vitest";
import { COMPOSER_SUBMIT_MODE, definePlugin } from "@/plugins/sdk";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import { submitComposer, type SubmitDeps } from "./submitComposer";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";

describe("submitComposer", () => {
  afterEach(resetKernelForTest);

  function deps(input: Partial<SubmitDeps>): SubmitDeps {
    return {
      value: "",
      clear: () => {},
      sendInput: () => true,
      images: [],
      pastes: [],
      recordHistory: () => {},
      canSend: () => true,
      ...input,
    };
  }

  it("is a no-op on empty / whitespace-only input", () => {
    const send = vi.fn(() => true);
    const clear = vi.fn();
    submitComposer(deps({ value: "   ", clear, sendInput: send }));
    expect(send).not.toHaveBeenCalled();
    expect(clear).not.toHaveBeenCalled();
  });

  it("forwards plain text as user input then clears", () => {
    const send = vi.fn(() => true);
    const clear = vi.fn();
    submitComposer(deps({ value: "hello", clear, sendInput: send }));
    expect(send).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "hello" }] });
    expect(clear).toHaveBeenCalledOnce();
  });

  it("keeps a draft intact while the Agent cannot accept a Run", () => {
    const send = vi.fn();
    const clear = vi.fn();
    const recordHistory = vi.fn();

    submitComposer(
      deps({
        value: "answer this later",
        canSend: () => false,
        clear,
        sendInput: send,
        recordHistory,
      }),
    );

    expect(send).not.toHaveBeenCalled();
    expect(clear).not.toHaveBeenCalled();
    expect(recordHistory).not.toHaveBeenCalled();
  });

  it("keeps a draft intact when admission changes between render and submit", () => {
    const send = vi.fn(() => false);
    const clear = vi.fn();
    const recordHistory = vi.fn();

    submitComposer(
      deps({
        value: "do not lose this race",
        canSend: () => true,
        clear,
        sendInput: send,
        recordHistory,
      }),
    );

    expect(send).toHaveBeenCalledOnce();
    expect(clear).not.toHaveBeenCalled();
    expect(recordHistory).not.toHaveBeenCalled();
  });

  it("lets one active execution mode claim the existing draft and accept it authoritatively", async () => {
    const submit = vi.fn((context: { accept(): void }) => context.accept());
    await loadPluginsForTest(
      definePlugin({
        name: "test.submit.mode",
        setup: (ctx) => {
          ctx.contribute(COMPOSER_SUBMIT_MODE, {
            id: "goal",
            matches: () => true,
            submit,
          });
        },
      }),
    );
    const send = vi.fn(() => true);
    const clear = vi.fn();
    const recordHistory = vi.fn();

    submitComposer(deps({ value: "ship it", clear, sendInput: send, recordHistory }));

    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        rawText: "ship it",
        text: "ship it",
        body: "ship it",
        slash: null,
        hasImages: false,
        hasPastes: false,
        accept: expect.any(Function),
        clear: expect.any(Function),
      }),
    );
    expect(send).not.toHaveBeenCalled();
    expect(recordHistory).toHaveBeenCalledWith("ship it");
    expect(clear).toHaveBeenCalledOnce();
  });

  it("keeps the draft when a claimed execution mode has not accepted it", async () => {
    await loadPluginsForTest(
      definePlugin({
        name: "test.submit.pending-mode",
        setup: (ctx) => {
          ctx.contribute(COMPOSER_SUBMIT_MODE, {
            id: "goal",
            matches: () => true,
            submit: () => {},
          });
        },
      }),
    );
    const send = vi.fn(() => true);
    const clear = vi.fn();

    submitComposer(deps({ value: "ship it", clear, sendInput: send }));

    expect(send).not.toHaveBeenCalled();
    expect(clear).not.toHaveBeenCalled();
  });

  it("routes a registered slash command to its handler — sendInput not called", async () => {
    const run = vi.fn();
    await loadPluginsForTest(
      definePlugin({
        name: "test.submit.slash",
        setup: (ctx) => {
          ctx.contribute(SLASH_COMMAND, { description: "echo", run }, { key: "/echo" });
        },
      }),
    );
    const send = vi.fn(() => true);
    const clear = vi.fn();
    submitComposer(deps({ value: "/echo hi there", clear, sendInput: send }));
    // The slash handler gets a text→input adapter, not sendInput itself.
    expect(run).toHaveBeenCalledWith({ args: "hi there", send: expect.any(Function) });
    expect(send).not.toHaveBeenCalled();
    expect(clear).toHaveBeenCalledOnce();
  });

  it("falls back to sendInput for an unknown slash command", () => {
    const send = vi.fn(() => true);
    submitComposer(deps({ value: "/unknown args", sendInput: send }));
    expect(send).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "/unknown args" }] });
  });

  it("folds pasted-text attachments into the message below the typed text", () => {
    const send = vi.fn(() => true);
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
    const send = vi.fn(() => true);
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
