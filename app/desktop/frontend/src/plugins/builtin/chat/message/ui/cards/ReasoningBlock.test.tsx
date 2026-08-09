import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ReasoningBlock } from "./ReasoningBlock";

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
});
