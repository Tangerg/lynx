import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { QuestionCard } from "./QuestionCard";

vi.mock("@/plugins/builtin/agent/public/hitl", () => ({
  useQuestionAnswer: () => ({ submit: vi.fn(), pending: false }),
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

describe("QuestionCard choice semantics", () => {
  it("exposes single-choice answers as a labeled radio group", () => {
    render(
      <QuestionCard
        status="requires-action"
        runId="run-1"
        itemId="question-1"
        questions={[
          {
            type: "choice",
            header: "Gate",
            prompt: "Which gate should run next?",
            multiple: false,
            allowCustom: false,
            options: [
              { label: "Race detector", description: "Exercise concurrency paths." },
              { label: "Visual suite", description: "Verify visual states." },
            ],
          },
        ]}
      />,
    );

    expect(screen.getByRole("radiogroup", { name: "Which gate should run next?" })).toBeTruthy();
    expect(screen.getByRole("radio", { name: /Race detector/ }).getAttribute("aria-checked")).toBe(
      "false",
    );
  });

  it("exposes multi-choice answers as checkboxes", () => {
    render(
      <QuestionCard
        status="requires-action"
        runId="run-1"
        itemId="question-2"
        questions={[
          {
            type: "choice",
            header: "Checks",
            prompt: "Which checks should run?",
            multiple: true,
            allowCustom: false,
            options: [
              { label: "Race detector", description: "" },
              { label: "Visual suite", description: "" },
            ],
          },
        ]}
      />,
    );

    expect(screen.getAllByRole("checkbox")).toHaveLength(2);
    expect(
      screen.getByRole("checkbox", { name: "Race detector" }).getAttribute("aria-checked"),
    ).toBe("false");
  });

  it("moves and selects within a radio group with the arrow keys", () => {
    render(
      <QuestionCard
        status="requires-action"
        runId="run-1"
        itemId="question-3"
        questions={[
          {
            type: "choice",
            header: "Gate",
            prompt: "Which gate should run next?",
            multiple: false,
            allowCustom: false,
            options: [
              { label: "Race detector", description: "" },
              { label: "Visual suite", description: "" },
            ],
          },
        ]}
      />,
    );

    const first = screen.getByText("Race detector").closest("button");
    const second = screen.getByText("Visual suite").closest("button");
    expect(first).not.toBeNull();
    expect(second).not.toBeNull();

    first!.focus();
    fireEvent.keyDown(first!, { key: "ArrowDown" });

    expect(second!.getAttribute("aria-checked")).toBe("true");
    expect(document.activeElement).toBe(second);
    expect((screen.getByRole("button", { name: "Submit" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("renders only the selected single-choice preview in a comparison sidecar", () => {
    render(
      <QuestionCard
        status="requires-action"
        runId="run-1"
        itemId="question-4"
        questions={[
          {
            type: "choice",
            header: "Gate",
            prompt: "Which gate should run next?",
            multiple: false,
            allowCustom: false,
            options: [
              { label: "Race detector", description: "", preview: "race-preview" },
              { label: "Visual suite", description: "", preview: "visual-preview" },
            ],
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByText("Race detector").closest("button")!);

    expect(screen.getByRole("region", { name: "Race detector" }).textContent).toContain(
      "race-preview",
    );
    expect(screen.queryByText("visual-preview")).toBeNull();
  });

  it("gives a freeform question a multiline answer surface", () => {
    render(
      <QuestionCard
        status="requires-action"
        runId="run-1"
        itemId="question-5"
        questions={[
          {
            type: "text",
            header: "Context",
            prompt: "Describe the constraints for this change.",
          },
        ]}
      />,
    );

    const answer = screen.getByRole("textbox", {
      name: "Describe the constraints for this change.",
    });
    expect(answer.tagName).toBe("TEXTAREA");
    expect(answer.getAttribute("rows")).toBe("4");
  });

  it("preserves multiline answers when the question settles", () => {
    render(
      <QuestionCard
        status="complete"
        runId="run-1"
        itemId="question-6"
        answered
        answers={[["First constraint\nSecond constraint"]]}
        questions={[
          {
            type: "text",
            header: "Context",
            prompt: "Describe the constraints for this change.",
          },
        ]}
      />,
    );

    const answer = [...document.querySelectorAll("div")].find(
      (element) =>
        element.children.length === 0 &&
        element.textContent === "First constraint\nSecond constraint",
    );
    expect(answer).toBeTruthy();
    expect(answer!.className).toContain("whitespace-pre-wrap");
  });
});
