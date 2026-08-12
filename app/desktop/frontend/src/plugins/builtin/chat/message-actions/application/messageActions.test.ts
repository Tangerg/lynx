import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/plugins/builtin/agent/public/viewState";

const model = vi.hoisted(() => ({
  conversation: null as { sessionId: string; messages: Message[] } | null,
  rollback: vi.fn(),
  send: vi.fn(),
  replaceDraft: vi.fn(),
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  activeAgentConversation: () => model.conversation,
  forkAgentSessionAtRun: vi.fn(),
  rollbackSessionToBeforeRun: model.rollback,
  sendToAgentSession: model.send,
}));
vi.mock("@/plugins/builtin/chat/composer/public/draft", () => ({
  replaceComposerDraft: model.replaceDraft,
}));
vi.mock("@/plugins/sdk", () => ({
  notifyError: vi.fn(),
  notifyInfo: vi.fn(),
}));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));
vi.mock("@/lib/rpcErrors", () => ({ describeRpcError: () => undefined }));

import { editAndRerunMessage, regenerateMessage } from "./messageActions";

beforeEach(() => {
  model.rollback.mockReset();
  model.send.mockReset();
  model.replaceDraft.mockReset();
  model.send.mockReturnValue(true);
  const prompt = message({
    id: "user_1",
    role: "user",
    runId: "run_1",
    blocks: [{ kind: "text", status: "complete", text: "stale projection" }],
  });
  model.conversation = {
    sessionId: "ses_1",
    messages: [prompt, message({ id: "assistant_1", runId: "run_1" })],
  };
});

describe("message history actions", () => {
  it("regenerates from the Runtime-authored dropped input", async () => {
    model.rollback.mockResolvedValue({
      status: "committed",
      userInput: {
        parts: [
          { kind: "text", text: "authoritative prompt" },
          { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      },
    });

    regenerateMessage(model.conversation!.messages[1]!);

    await vi.waitFor(() =>
      expect(model.send).toHaveBeenCalledWith("ses_1", {
        parts: [
          { kind: "text", text: "authoritative prompt" },
          { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      }),
    );
  });

  it("does not run a second follow-up for a joined history rewrite", async () => {
    model.rollback.mockResolvedValue({ status: "inFlight" });

    regenerateMessage(model.conversation!.messages[1]!);

    await vi.waitFor(() => expect(model.rollback).toHaveBeenCalledOnce());
    expect(model.send).not.toHaveBeenCalled();
  });

  it("prefills edit-and-rerun from the Runtime-authored dropped input", async () => {
    model.rollback.mockResolvedValue({
      status: "committed",
      userInput: {
        parts: [
          { kind: "text", text: "canonical edit" },
          { kind: "image", mime: "image/jpeg", data: "anBlZw==" },
        ],
      },
    });

    editAndRerunMessage(model.conversation!.messages[0]!);

    await vi.waitFor(() =>
      expect(model.replaceDraft).toHaveBeenCalledWith({
        text: "canonical edit",
        images: [{ mime: "image/jpeg", data: "anBlZw==" }],
      }),
    );
  });
});

function message({ runId = null, ...overrides }: Partial<Message>): Message {
  return {
    blocks: [],
    id: "message",
    role: "assistant",
    createdAt: "2026-08-12T00:00:00.000Z",
    runId,
    ...overrides,
  };
}
