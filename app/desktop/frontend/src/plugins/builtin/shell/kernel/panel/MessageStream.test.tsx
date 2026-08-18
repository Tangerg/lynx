import type { PropsWithChildren } from "react";
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { BlockCtx } from "@/plugins/builtin/chat/message/public/rendering";

const { rootRunning } = vi.hoisted(() => ({ rootRunning: { current: true } }));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useIsCurrentRootRunning: () => rootRunning.current,
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

vi.mock("@/lib/appearance", () => ({ useMotionOff: () => false }));

vi.mock("motion/react", () => ({
  AnimatePresence: ({ children }: PropsWithChildren) => children,
  motion: {
    div: ({ children }: PropsWithChildren) => <div>{children}</div>,
  },
}));

vi.mock("use-stick-to-bottom", () => {
  const StickToBottom = Object.assign(({ children }: PropsWithChildren) => <div>{children}</div>, {
    Content: ({ children }: PropsWithChildren) => <div>{children}</div>,
  });
  return {
    StickToBottom,
    useStickToBottomContext: () => ({
      isAtBottom: true,
      scrollRef: { current: null },
      scrollToBottom: vi.fn(),
    }),
  };
});

import { MessageStream } from "./MessageStream";

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

describe("MessageStream terminal footer materialization", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("data-motion");
  });

  it("keeps the Run outcome out of layout until the terminal visible generation settles", async () => {
    rootRunning.current = true;
    const { rerender } = render(
      <MessageStream rows={[transcriptRow("running")]} ctx={CTX} sessionId="session-footer" />,
    );
    expect(screen.queryByTestId("root-run-outcome")).toBeNull();

    rootRunning.current = false;
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
});
