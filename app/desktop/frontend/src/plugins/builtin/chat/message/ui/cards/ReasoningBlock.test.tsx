import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ReasoningBlock } from "./ReasoningBlock";

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

describe("ReasoningBlock disclosure policy", () => {
  it("turns the first user toggle into an explicit override of the automatic state", () => {
    render(<ReasoningBlock text="Hidden rationale" status="complete" />);
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
    render(<ReasoningBlock text="Inspect the protocol boundary" status="running" />);

    const activity = screen.getByRole("button", { name: /Thinking/ }).closest("[data-shell]");
    const body = screen.getByRole("region");

    expect(activity?.getAttribute("data-shell")).toBe("line");
    expect(body.className).toContain("border-l");
    expect(body.className).toContain("pl-6");
  });
});
