import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { AgentMessagePhase } from "@/plugins/builtin/agent/public/viewState";
import type { BlockCtx } from "./BlockRenderer";
import { MessageBlock } from "./MessageBlock";
import { MessageVisibleMaterialOwner } from "./messageVisibleMaterial";

const { renderActionSlot } = vi.hoisted(() => ({ renderActionSlot: vi.fn() }));

vi.mock("@/plugins/host/Slot", () => ({
  Slot: ({ name }: { name: string }) => {
    renderActionSlot(name);
    return <div data-testid={name} />;
  },
}));

const CTX: BlockCtx = {
  onSelectTool: vi.fn(),
  expandedIds: new Set(),
  onToggleExpand: vi.fn(),
  textReveal: "smooth",
};

function row(
  status: "running" | "complete",
  text = "A deliberately long answer whose reveal still has a visible backlog.",
  role: "user" | "assistant" = "assistant",
  phase?: AgentMessagePhase,
): TranscriptRow {
  return {
    message: {
      id: "assistant-visible-material",
      runId: "run-visible-material",
      role,
      ...(phase ? { phase } : {}),
      createdAt: "2026-08-18T14:32:00.000Z",
      blocks: [
        {
          kind: "text",
          itemId: "answer-visible-material",
          text,
          status,
        },
      ],
    },
    runOwner: { kind: "owned", runId: "run-visible-material", status: "finished" },
    facts: { toolCalls: {}, delegatedRuns: {} },
  };
}

describe("MessageBlock turn identity", () => {
  it("keeps role identity semantic without adding visible chrome to every turn", () => {
    const { container, rerender } = render(
      <MessageBlock
        row={row("complete", "Question", "user")}
        ctx={CTX}
        sessionId="session-turn-identity"
        isLast={false}
        isRunning={false}
      />,
    );

    expect(screen.getByRole("heading", { name: "You" }).classList.contains("sr-only")).toBe(true);
    expect(container.querySelector(".font-mono.tabular-nums")).toBeNull();

    rerender(
      <MessageBlock
        row={row("complete", "Answer")}
        ctx={CTX}
        sessionId="session-turn-identity"
        isLast
        isRunning={false}
      />,
    );

    expect(screen.getByRole("heading", { name: "Assistant" }).classList.contains("sr-only")).toBe(
      true,
    );
    expect(container.querySelector(".font-mono.tabular-nums")).toBeNull();
  });

  it("marks only the human turn as the user-message bubble", () => {
    const { container, rerender } = render(
      <MessageBlock
        row={row("complete", "中文与 English 混排", "user")}
        ctx={CTX}
        sessionId="session-user-bubble"
        isLast={false}
        isRunning={false}
      />,
    );

    expect(container.querySelector("[data-user-message-bubble]")?.textContent).toContain(
      "中文与 English 混排",
    );

    rerender(
      <MessageBlock
        row={row("complete", "Assistant answer")}
        ctx={CTX}
        sessionId="session-user-bubble"
        isLast
        isRunning={false}
      />,
    );

    expect(container.querySelector("[data-user-message-bubble]")).toBeNull();
  });
});

beforeEach(() => {
  renderActionSlot.mockClear();
  document.documentElement.removeAttribute("data-motion");
});

afterEach(() => {
  document.documentElement.removeAttribute("data-motion");
});

describe("MessageBlock action materialization", () => {
  it("keeps work commentary free of terminal message actions", async () => {
    document.documentElement.setAttribute("data-motion", "off");
    const { rerender } = render(
      <MessageBlock
        row={row("complete", "Inspecting the project.", "assistant", "commentary")}
        ctx={CTX}
        sessionId="session-message-phase"
        isLast
        isRunning={false}
      />,
    );

    expect(screen.queryByTestId("message.actions")).toBeNull();

    rerender(
      <MessageBlock
        row={row("complete", "Inspection complete.", "assistant", "finalAnswer")}
        ctx={CTX}
        sessionId="session-message-phase"
        isLast
        isRunning={false}
      />,
    );

    expect(await screen.findByTestId("message.actions")).toBeTruthy();
  });

  it("revokes terminal actions before a late accepted text generation enters layout", async () => {
    document.documentElement.setAttribute("data-motion", "off");
    const { rerender } = render(
      <MessageBlock
        row={row("complete", "Settled answer.")}
        ctx={CTX}
        sessionId="session-visible-material"
        isLast
        isRunning={false}
      />,
    );
    expect(await screen.findByTestId("message.actions")).toBeTruthy();
    const predecessorActionRenders = renderActionSlot.mock.calls.length;

    document.documentElement.removeAttribute("data-motion");
    rerender(
      <MessageBlock
        row={row("complete", "Settled answer with a late accepted continuation.")}
        ctx={CTX}
        sessionId="session-visible-material"
        isLast
        isRunning={false}
      />,
    );

    expect(screen.queryByTestId("message.actions")).toBeNull();
    expect(renderActionSlot).toHaveBeenCalledTimes(predecessorActionRenders);
  });

  it("does not mount terminal controls when root attention drops before the exact Run settles", () => {
    const exactRunStillStreaming = {
      ...row("complete"),
      runOwner: { kind: "owned", runId: "run-visible-material", status: "running" },
    } as TranscriptRow;

    render(
      <MessageBlock
        row={exactRunStillStreaming}
        ctx={CTX}
        sessionId="session-visible-material"
        isLast
        isRunning={false}
      />,
    );

    expect(screen.queryByTestId("message.actions")).toBeNull();
  });

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
    const generation = {};

    owner.observe(predecessor, generation, false);
    owner.observe(successor, generation, false);
    owner.retire(predecessor);

    expect(owner.actionsMaterialization("settled", generation)).toBe("active");

    owner.observe(successor, generation, true);
    expect(owner.actionsMaterialization("settled", generation)).toBe("settled");
  });

  it("does not lend predecessor settlement to a successor accepted generation", () => {
    const owner = new MessageVisibleMaterialOwner("session-owner", "message-owner");
    const projection = Symbol("projection");
    const predecessor = {};
    const successor = {};

    owner.observe(projection, predecessor, true);

    expect(owner.actionsMaterialization("settled", successor)).toBe("active");

    owner.observe(projection, successor, true);
    expect(owner.actionsMaterialization("settled", successor)).toBe("settled");
  });

  it("does not publish when an active projection advances between active generations", () => {
    const owner = new MessageVisibleMaterialOwner("session-owner", "message-owner");
    const projection = Symbol("projection");
    const changed = vi.fn();
    owner.subscribe(changed);

    owner.observe(projection, {}, false);
    changed.mockClear();
    owner.observe(projection, {}, false);

    expect(changed).not.toHaveBeenCalled();
  });
});
