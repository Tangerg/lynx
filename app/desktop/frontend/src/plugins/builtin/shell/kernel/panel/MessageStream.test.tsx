import { createRef, type MutableRefObject, type PropsWithChildren } from "react";
import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { BlockCtx } from "@/plugins/builtin/chat/message/public/rendering";

const { root, stick } = vi.hoisted(() => ({
  root: {
    current: {
      running: true,
      terminalTurnIndex: () => -1,
    },
  },
  stick: {
    presentationAtBottom: true,
    lockedToBottom: true,
  },
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useCurrentRootMaterial: () => root.current,
}));

vi.mock("@/plugins/builtin/chat/message/public/rendering", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/plugins/builtin/chat/message/public/rendering")>();
  return {
    ...actual,
    RootRunOutcome: () => <div data-testid="root-run-outcome" />,
  };
});

vi.mock("@/plugins/host/Slot", () => ({
  Slot: ({ name }: { name: string }) => <div data-testid={name} />,
}));

vi.mock("motion/react", () => ({
  AnimatePresence: ({ children }: PropsWithChildren) => children,
  motion: {
    div: ({ children, "data-turn-id": turnId }: PropsWithChildren<{ "data-turn-id"?: string }>) => (
      <div data-turn-id={turnId}>{children}</div>
    ),
  },
}));

vi.mock("use-stick-to-bottom", () => {
  const context = {
    get isAtBottom() {
      return stick.presentationAtBottom;
    },
    scrollRef: { current: null as HTMLDivElement | null },
    scrollToBottom: vi.fn(),
    state: {
      get isAtBottom() {
        return stick.lockedToBottom;
      },
      get calculatedTargetScrollTop() {
        const viewport = context.scrollRef.current;
        return viewport ? Math.max(viewport.scrollHeight - viewport.clientHeight - 1, 0) : 0;
      },
    },
  };
  const StickToBottom = Object.assign(
    ({
      children,
      contextRef,
    }: PropsWithChildren<{ contextRef?: MutableRefObject<typeof context | null> }>) => {
      if (contextRef) contextRef.current = context;
      return <div>{children}</div>;
    },
    {
      Content: ({ children, scrollClassName }: PropsWithChildren<{ scrollClassName?: string }>) => (
        <div ref={(node) => (context.scrollRef.current = node)} className={scrollClassName}>
          {children}
        </div>
      ),
    },
  );
  return {
    StickToBottom,
    useStickToBottomContext: () => context,
  };
});

import { MessageStream, type MessageStreamController } from "./MessageStream";

const CTX: BlockCtx = {
  onSelectTool: vi.fn(),
  expandedIds: new Set(),
  onToggleExpand: vi.fn(),
  textReveal: "smooth",
};

function transcriptRow(status: "running" | "complete"): TranscriptRow {
  const runStatus = status === "running" ? "running" : "finished";
  return {
    message: {
      id: "assistant-terminal-footer",
      runId: "run-terminal-footer",
      role: "assistant",
      blocks: [
        {
          kind: "text",
          itemId: "answer-terminal-footer",
          text: "A long terminal answer whose visible projection still has a reveal backlog.",
          status,
        },
      ],
    },
    runOwner: { kind: "owned", runId: "run-terminal-footer", status: runStatus },
    facts: { toolCalls: {}, delegatedRuns: {} },
  };
}

function optimisticUserRow(): TranscriptRow {
  return {
    message: {
      id: "local-successor-user",
      runId: null,
      role: "user",
      blocks: [
        {
          kind: "text",
          itemId: "local-successor-text",
          text: "Start another run",
          status: "complete",
        },
      ],
    },
    runOwner: { kind: "unassigned" },
    facts: { toolCalls: {}, delegatedRuns: {} },
  };
}

describe("MessageStream terminal footer materialization", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("data-motion");
    root.current = { running: true, terminalTurnIndex: () => -1 };
  });

  it("keeps the Run outcome out of layout until the terminal visible generation settles", async () => {
    root.current = { running: true, terminalTurnIndex: () => -1 };
    const { rerender } = render(
      <MessageStream rows={[transcriptRow("running")]} ctx={CTX} sessionId="session-footer" />,
    );
    expect(screen.queryByTestId("root-run-outcome")).toBeNull();

    root.current = { running: false, terminalTurnIndex: () => 0 };
    rerender(
      <MessageStream rows={[transcriptRow("complete")]} ctx={CTX} sessionId="session-footer" />,
    );

    expect(screen.queryByTestId("root-run-outcome")).toBeNull();

    document.documentElement.setAttribute("data-motion", "off");
    rerender(
      <MessageStream rows={[transcriptRow("complete")]} ctx={CTX} sessionId="session-footer" />,
    );

    expect(await screen.findByTestId("root-run-outcome")).toBeTruthy();
  });

  it("keeps a finished Run outcome with its exact turn while a successor user message is unassigned", () => {
    root.current = { running: false, terminalTurnIndex: () => 0 };

    render(
      <MessageStream
        rows={[transcriptRow("complete"), optimisticUserRow()]}
        ctx={CTX}
        sessionId="session-footer"
      />,
    );

    expect(screen.getByTestId("root-run-outcome").closest("[data-turn-id]")?.dataset.turnId).toBe(
      "assistant-terminal-footer",
    );
  });
});

describe("MessageStream initial bottom reconciliation", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    stick.presentationAtBottom = true;
    stick.lockedToBottom = true;
  });

  it("does not take the transcript back after the reader scrolls away", () => {
    vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame", "performance"] });
    const controllerRef = createRef<MessageStreamController>();

    render(
      <MessageStream
        rows={[transcriptRow("running")]}
        ctx={CTX}
        sessionId="session-reader-scroll"
        controllerRef={controllerRef}
      />,
    );

    const viewport = document.querySelector<HTMLDivElement>(".msg-scroll-viewport");
    expect(viewport).not.toBeNull();
    Object.defineProperties(viewport, {
      clientHeight: { configurable: true, value: 400 },
      scrollHeight: { configurable: true, value: 1_000 },
      scrollTop: { configurable: true, value: 0, writable: true },
    });

    act(() => controllerRef.current?.settleInitialBottom());
    expect(viewport?.scrollTop).toBe(599);

    // A wheel/pointer interaction makes the reading position user-owned. The
    // mount reconciliation may not keep writing the old tail on later frames.
    if (viewport) viewport.scrollTop = 240;
    act(() => vi.advanceTimersToNextFrame());

    expect(viewport?.scrollTop).toBe(240);
  });

  it("does not confuse the near-bottom presentation state with the reader-owned follow lock", () => {
    const mutationCallbacks: MutationCallback[] = [];
    class ControlledMutationObserver implements MutationObserver {
      constructor(callback: MutationCallback) {
        mutationCallbacks.push(callback);
      }

      disconnect() {}
      observe() {}
      takeRecords(): MutationRecord[] {
        return [];
      }
    }
    vi.stubGlobal("MutationObserver", ControlledMutationObserver);

    render(
      <MessageStream
        rows={[transcriptRow("running")]}
        ctx={CTX}
        sessionId="session-wheel-escape"
      />,
    );

    const viewport = document.querySelector<HTMLDivElement>(".msg-scroll-viewport");
    expect(viewport).not.toBeNull();
    Object.defineProperties(viewport, {
      clientHeight: { configurable: true, value: 400 },
      scrollHeight: { configurable: true, value: 1_000 },
      scrollTop: { configurable: true, value: 540, writable: true },
    });

    // The library deliberately reports `isAtBottom=true` inside its 70px
    // presentation threshold so the jump button stays quiet. A wheel-up escape,
    // however, has already released the underlying follow lock. Streaming DOM
    // growth must respect that raw lock immediately instead of snapping the
    // reader through the remaining near-bottom band.
    stick.presentationAtBottom = true;
    stick.lockedToBottom = false;
    mutationCallbacks[0]?.([], {} as MutationObserver);

    expect(viewport?.scrollTop).toBe(540);
  });
});
