import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { BlockCtx } from "./BlockRenderer";
import { MessageBlock } from "./MessageBlock";
import { MessageVisibleMaterialOwner } from "./messageVisibleMaterial";

vi.mock("@/plugins/host/Slot", () => ({
  Slot: ({ name }: { name: string }) => <div data-testid={name} />,
}));

const CTX: BlockCtx = {
  onSelectTool: vi.fn(),
  expandedIds: new Set(),
  onToggleExpand: vi.fn(),
  textReveal: "smooth",
};

function row(status: "running" | "complete"): TranscriptRow {
  return {
    message: {
      id: "assistant-visible-material",
      runId: "run-visible-material",
      role: "assistant",
      blocks: [
        {
          kind: "text",
          itemId: "answer-visible-material",
          text: "A deliberately long answer whose reveal still has a visible backlog.",
          status,
        },
      ],
    },
    facts: { toolCalls: {}, delegatedRuns: {} },
  };
}

describe("MessageBlock action materialization", () => {
  it("keeps terminal actions absent until the visible text generation has drained", () => {
    const { rerender } = render(
      <MessageBlock
        row={row("running")}
        ctx={CTX}
        sessionId="session-visible-material"
        isLast
        isRunning
      />,
    );

    expect(screen.queryByTestId("message.actions")).toBeNull();

    rerender(
      <MessageBlock
        row={row("complete")}
        ctx={CTX}
        sessionId="session-visible-material"
        isLast
        isRunning={false}
      />,
    );

    // The source Item and Run have settled, but useStreamReveal still owns a
    // backlog from the mounted streaming generation. Mounting the action row at
    // this boundary makes it visibly chase the growing answer tail.
    expect(screen.queryByTestId("message.actions")).toBeNull();
  });

  it("publishes terminal actions once the visible generation catches up", async () => {
    const { rerender } = render(
      <MessageBlock
        row={row("running")}
        ctx={CTX}
        sessionId="session-visible-material"
        isLast
        isRunning
      />,
    );

    // Reduced motion makes the already-mounted reveal publish its entire
    // accepted tail on the terminal render; the owner must then retire the
    // presenting generation instead of suppressing actions forever.
    document.documentElement.setAttribute("data-motion", "off");
    try {
      rerender(
        <MessageBlock
          row={row("complete")}
          ctx={CTX}
          sessionId="session-visible-material"
          isLast
          isRunning={false}
        />,
      );

      expect(await screen.findByTestId("message.actions")).toBeTruthy();
    } finally {
      document.documentElement.removeAttribute("data-motion");
    }
  });
});

describe("MessageVisibleMaterialOwner", () => {
  it("does not let predecessor teardown retire a successor projection", () => {
    const owner = new MessageVisibleMaterialOwner("session-owner", "message-owner");
    const predecessor = Symbol("predecessor");
    const successor = Symbol("successor");

    owner.observe(predecessor, false);
    owner.observe(successor, false);
    owner.retire(predecessor);

    expect(owner.actionsMaterialization("settled")).toBe("active");

    owner.observe(successor, true);
    expect(owner.actionsMaterialization("settled")).toBe("settled");
  });
});
