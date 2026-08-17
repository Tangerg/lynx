import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ConversationArchiveOwner,
  exportConversationMarkdown,
  importConversationJson,
} from "./conversationExport";
import type { ConversationArchiveGateway } from "./ports/conversationArchiveGateway";
import type { FileTransferPort } from "./ports/fileTransfer";

const mocks = vi.hoisted(() => ({
  activeSessionId: "session-current" as string | undefined,
  getActiveConversationSnapshot: vi.fn(() => ({
    messages: [],
    timeline: [],
    toolCalls: [],
  })),
  invalidateAgentSessions: vi.fn().mockResolvedValue(undefined),
  notifyError: vi.fn(),
  rehydrateSessionView: vi.fn().mockResolvedValue(undefined),
  selectAgentSession: vi.fn(),
  success: vi.fn(),
}));

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  runtimeCapability: () => true,
}));

vi.mock("@/plugins/builtin/agent/public/conversation", () => ({
  getActiveConversationSnapshot: mocks.getActiveConversationSnapshot,
}));

vi.mock("@/plugins/builtin/agent/public/messageContent", () => ({
  flattenMarkdown: () => "",
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  getActiveSessionId: () => mocks.activeSessionId,
  invalidateAgentSessions: mocks.invalidateAgentSessions,
  rehydrateSessionView: mocks.rehydrateSessionView,
  selectAgentSession: mocks.selectAgentSession,
}));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  lookupExtensionByKey: vi.fn(),
  notifyError: mocks.notifyError,
}));

vi.mock("sonner", () => ({ toast: { success: mocks.success } }));

const disposers: Array<() => void> = [];
let download = vi.fn<(filename: string, content: string, mime: string) => void>();
let files: FileTransferPort | null;

beforeEach(() => {
  mocks.activeSessionId = "session-current";
  mocks.getActiveConversationSnapshot.mockClear();
  mocks.invalidateAgentSessions.mockReset().mockResolvedValue(undefined);
  mocks.notifyError.mockReset();
  mocks.rehydrateSessionView.mockReset().mockResolvedValue(undefined);
  mocks.selectAgentSession.mockReset();
  mocks.success.mockReset();
  download = vi.fn<(filename: string, content: string, mime: string) => void>();
  files = null;
});

afterEach(() => {
  for (const dispose of disposers.splice(0).reverse()) dispose();
  vi.restoreAllMocks();
});

describe("conversation archive generation", () => {
  it("retires a picker before its continuation can borrow the successor gateway", async () => {
    const picker = deferred<string | null>();
    const successorImport = vi.fn().mockResolvedValue({ id: "successor-session" });
    installFiles({ download, pickText: () => picker.promise });
    installGateway({ importConversation: vi.fn() });

    const retired = importConversationJson();
    await drainMicrotasks();
    installGateway({ importConversation: successorImport });
    const hasSettled = observedSettlement(retired);
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    picker.resolve(validArtifact("old-session"));
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(successorImport).not.toHaveBeenCalled();
    expect(mocks.rehydrateSessionView).not.toHaveBeenCalled();
    expect(mocks.selectAgentSession).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("settles an in-flight import at replacement and rejects every late product effect", async () => {
    const response = deferred<{ id: string; title?: string }>();
    const retiredImport = vi.fn(() => response.promise);
    installFiles({ download, pickText: () => Promise.resolve(validArtifact("old-session")) });
    installGateway({ importConversation: retiredImport });

    const retired = importConversationJson();
    await vi.waitFor(() => expect(retiredImport).toHaveBeenCalledOnce());
    installGateway({ importConversation: vi.fn().mockResolvedValue({ id: "successor-session" }) });
    const hasSettled = observedSettlement(retired);
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    response.resolve({ id: "old-session", title: "Old" });
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(mocks.rehydrateSessionView).not.toHaveBeenCalled();
    expect(mocks.invalidateAgentSessions).not.toHaveBeenCalled();
    expect(mocks.selectAgentSession).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("does not turn a retired export failure into a successor local download", async () => {
    const response = deferred<never>();
    const retiredExport = vi.fn(() => response.promise);
    installFiles({ download, pickText: vi.fn() });
    installGateway({ exportConversation: retiredExport });

    const retired = exportConversationMarkdown();
    await vi.waitFor(() => expect(retiredExport).toHaveBeenCalledOnce());
    installGateway({
      exportConversation: vi.fn().mockResolvedValue({ format: "md", markdown: "successor" }),
    });
    const hasSettled = observedSettlement(retired);
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    response.reject(new Error("old connection closed"));
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(mocks.getActiveConversationSnapshot).toHaveBeenCalledOnce();
    expect(download).not.toHaveBeenCalled();
  });

  it("retires the import while rehydrate is pending before query repair or navigation", async () => {
    const hydration = deferred<void>();
    mocks.rehydrateSessionView.mockReturnValueOnce(hydration.promise);
    installFiles({ download, pickText: () => Promise.resolve(validArtifact("imported")) });
    const owner = installGateway({
      importConversation: vi.fn().mockResolvedValue({ id: "imported", title: "Imported" }),
    });

    const retired = importConversationJson();
    const hasSettled = observedSettlement(retired);
    await vi.waitFor(() => expect(mocks.rehydrateSessionView).toHaveBeenCalledWith("imported"));
    owner.replaceRuntimeGeneration();
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    hydration.resolve();
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(mocks.invalidateAgentSessions).not.toHaveBeenCalled();
    expect(mocks.selectAgentSession).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("retires query repair before it can publish old import navigation", async () => {
    const repair = deferred<void>();
    mocks.invalidateAgentSessions.mockReturnValueOnce(repair.promise);
    installFiles({ download, pickText: () => Promise.resolve(validArtifact("imported")) });
    const owner = installGateway({
      importConversation: vi.fn().mockResolvedValue({ id: "imported", title: "Imported" }),
    });

    const retired = importConversationJson();
    const hasSettled = observedSettlement(retired);
    await vi.waitFor(() => expect(mocks.invalidateAgentSessions).toHaveBeenCalledOnce());
    owner.replaceRuntimeGeneration();
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    repair.resolve();
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(mocks.selectAgentSession).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("publishes an accepted import even when Session collection repair fails", async () => {
    mocks.invalidateAgentSessions.mockRejectedValueOnce(new Error("query unavailable"));
    installFiles({ download, pickText: () => Promise.resolve(validArtifact("imported")) });
    installGateway({
      importConversation: vi.fn().mockResolvedValue({ id: "imported", title: "Imported" }),
    });

    await importConversationJson();

    expect(mocks.rehydrateSessionView).toHaveBeenCalledWith("imported");
    expect(mocks.selectAgentSession).toHaveBeenCalledWith("imported");
    expect(mocks.success).toHaveBeenCalledOnce();
    expect(mocks.notifyError).not.toHaveBeenCalled();
  });

  it("owns repeated import intent as one picker and one mutation", async () => {
    const picker = deferred<string | null>();
    const importConversation = vi.fn().mockResolvedValue({ id: "imported" });
    const pickText = vi.fn(() => picker.promise);
    installFiles({ download, pickText });
    installGateway({ importConversation });

    const first = importConversationJson();
    const repeated = importConversationJson();

    expect(repeated).toBe(first);
    expect(pickText).toHaveBeenCalledOnce();
    picker.resolve(null);
    await Promise.all([first, repeated]);
    expect(importConversation).not.toHaveBeenCalled();
  });
});

function installGateway(gateway: Partial<ConversationArchiveGateway>): ConversationArchiveOwner {
  if (!files) throw new Error("test file transfer is not installed");
  const owner = ConversationArchiveOwner.install({
    gateway: gateway as ConversationArchiveGateway,
    files,
  });
  disposers.push(() => owner.dispose());
  return owner;
}

function installFiles(next: FileTransferPort): void {
  files = next;
}

function validArtifact(id: string): string {
  return JSON.stringify({
    version: 1,
    session: { id },
    messages: [],
    runs: [],
    items: [],
  });
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
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
