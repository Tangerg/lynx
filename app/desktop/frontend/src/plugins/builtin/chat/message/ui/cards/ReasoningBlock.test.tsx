import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReasoningBlock } from "./ReasoningBlock";

function renderReasoning(status: "running" | "complete", text: string) {
  return render(<ReasoningBlock text={text} status={status} />);
}

afterEach(() => {
  vi.useRealTimers();
});

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

  it("does not disguise Run cancellation as an Answer now activity action", () => {
    renderReasoning("running", "A predecessor renderer is still settling.");

    expect(screen.queryByRole("button", { name: /Answer now/ })).toBeNull();
  });

  it("does not invent reasoning duration from renderer mount time", () => {
    vi.useFakeTimers();
    renderReasoning("running", "A restored reasoning item is still streaming.");

    act(() => {
      vi.advanceTimersByTime(3_000);
    });

    expect(screen.queryByText("3s", { exact: true })).toBeNull();
  });
});
