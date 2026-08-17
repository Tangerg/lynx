import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { MessageContext } from "@/plugins/sdk/messageContext";
import { ReasoningBlock } from "./ReasoningBlock";

const agentRunCommands = vi.hoisted(() => ({
  cancelExact: vi.fn(),
  stopCurrent: vi.fn(),
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  cancelSessionRun: agentRunCommands.cancelExact,
  stopCurrentRootRun: agentRunCommands.stopCurrent,
}));

const MESSAGE: Message = {
  id: "reasoning-message",
  role: "assistant",
  runId: "run-predecessor",
  blocks: [],
};

function renderReasoning(status: "running" | "complete", text: string) {
  return render(
    <MessageContext.Provider value={{ sessionId: "session-predecessor", message: MESSAGE }}>
      <ReasoningBlock text={text} status={status} />
    </MessageContext.Provider>,
  );
}

describe("ReasoningBlock disclosure policy", () => {
  it("turns the first user toggle into an explicit override of the automatic state", () => {
    renderReasoning("complete", "Hidden rationale");
    const trigger = screen.getByRole("button", { name: /Thought/ });

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("region")).toBeNull();

    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("region").textContent).toContain("Hidden rationale");

    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });

  it("sets expanded reasoning apart as an indented aside instead of a card", () => {
    renderReasoning("running", "Inspect the protocol boundary");

    const activity = screen.getByRole("button", { name: /Thinking/ }).closest("[data-shell]");
    const body = screen.getByRole("region");

    expect(activity?.getAttribute("data-shell")).toBe("line");
    expect(body.className).toContain("border-l");
    expect(body.className).toContain("pl-6");
  });

  it("cancels the exact Session and Run presented by a predecessor renderer", () => {
    renderReasoning("running", "A predecessor renderer is still settling.");

    fireEvent.click(screen.getByRole("button", { name: /Answer now/ }));

    expect(agentRunCommands.cancelExact).toHaveBeenCalledWith({
      sessionId: "session-predecessor",
      runId: "run-predecessor",
    });
    expect(agentRunCommands.stopCurrent).not.toHaveBeenCalled();
  });
});
