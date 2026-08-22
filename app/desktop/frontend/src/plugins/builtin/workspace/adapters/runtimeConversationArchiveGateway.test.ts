import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { exportConversationMarkdown } from "../application/conversationExport";
import { installConversationArchiveGateway } from "./runtimeConversationArchiveGateway";

const mocks = vi.hoisted(() => ({
  download: vi.fn<(filename: string, content: string, mime: string) => void>(),
}));

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  runtimeCapability: () => true,
}));

vi.mock("@/plugins/builtin/agent/public/conversation", () => ({
  getActiveConversationSnapshot: () => ({ messages: [], timeline: [], toolCalls: [] }),
}));

vi.mock("@/plugins/builtin/agent/public/messageContent", () => ({
  flattenMarkdown: () => "",
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  getActiveSessionId: () => "session-current",
  invalidateAgentSessions: vi.fn().mockResolvedValue(undefined),
  rehydrateSessionView: vi.fn().mockResolvedValue(undefined),
  selectAgentSession: vi.fn(),
}));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  lookupExtensionByKey: vi.fn(),
  notifyError: vi.fn(),
}));

vi.mock("./browserFileTransfer", () => ({
  browserFileTransfer: () => ({ download: mocks.download, pickText: vi.fn() }),
}));

const installations: Array<ReturnType<typeof installConversationArchiveGateway>> = [];

beforeEach(() => mocks.download.mockReset());

afterEach(async () => {
  for (const installation of installations.splice(0).reverse()) installation.dispose();
  vi.restoreAllMocks();
  await resetContainer();
});

describe("runtimeConversationArchiveGateway", () => {
  it("binds a Host owner to the exact client installed at composition time", async () => {
    const retiredExport = vi.fn().mockResolvedValue({ format: "md", markdown: "retired" });
    const successorExport = vi.fn().mockResolvedValue({ format: "md", markdown: "successor" });
    setContainer({ client: () => clientWithExport(retiredExport) });
    installations.push(installConversationArchiveGateway());

    setContainer({ client: () => clientWithExport(successorExport) });
    await exportConversationMarkdown();

    expect(retiredExport).toHaveBeenCalledWith("session-current", "md");
    expect(successorExport).not.toHaveBeenCalled();
    expect(mocks.download).toHaveBeenCalledWith(
      expect.stringContaining("lyra-session-current-"),
      "retired",
      "text/markdown;charset=utf-8",
    );
  });

  it("retires an admitted export when the same Host observes a Runtime generation", async () => {
    const response = deferred<{ format: "md"; markdown: string }>();
    const exportConversation = vi
      .fn()
      .mockReturnValueOnce(response.promise)
      .mockResolvedValueOnce({ format: "md", markdown: "current" });
    setContainer({ client: () => clientWithExport(exportConversation) });
    const installation = installConversationArchiveGateway();
    installations.push(installation);

    const retired = exportConversationMarkdown();
    const hasSettled = observedSettlement(retired);
    await vi.waitFor(() => expect(exportConversation).toHaveBeenCalledOnce());
    installation.replaceRuntimeGeneration();
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    response.resolve({ format: "md", markdown: "retired" });
    await retired;
    expect(settledAtReplacement).toBe(true);
    expect(mocks.download).not.toHaveBeenCalled();

    await exportConversationMarkdown();
    expect(mocks.download).toHaveBeenCalledWith(
      expect.stringContaining("lyra-session-current-"),
      "current",
      "text/markdown;charset=utf-8",
    );
  });
});

function clientWithExport(exportConversation: ReturnType<typeof vi.fn>): LyraClient {
  return {
    sessions: { export: exportConversation },
  } as unknown as LyraClient;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function observedSettlement(operation: Promise<unknown>): () => boolean {
  let settled = false;
  void operation.then(
    () => {
      settled = true;
    },
    () => {
      settled = true;
    },
  );
  return () => settled;
}

async function drainMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index++) await Promise.resolve();
}
