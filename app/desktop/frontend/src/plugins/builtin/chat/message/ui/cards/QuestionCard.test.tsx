import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { QuestionCard } from "./QuestionCard";

const submitQuestion = vi.hoisted(() => vi.fn());

vi.mock("@/plugins/builtin/agent/public/hitl", () => ({
  useQuestionAnswer: () => ({ submit: submitQuestion, pending: false }),
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

describe("QuestionCard choice semantics", () => {
  beforeEach(() => {
    submitQuestion.mockClear();
  });

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
      "true",
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
    expect((screen.getByRole("button", { name: "Skip" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("uses the question as the only request heading and keeps option detail inline", () => {
    const { container } = render(
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
              {
                label: "Race detector",
                description: "Exercise concurrency paths.",
                preview: "race-preview",
              },
              {
                label: "Visual suite",
                description: "Verify visual states.",
                preview: "visual-preview",
              },
            ],
          },
        ]}
      />,
    );

    const surface = container.querySelector<HTMLElement>('[data-slot="question-request-surface"]');
    expect(surface).not.toBeNull();
    expect(
      within(surface!).getByRole("heading", { name: "Which gate should run next?" }),
    ).toBeTruthy();
    expect(within(surface!).getByRole("radio", { name: /Race detector/ }).textContent).toContain(
      "Exercise concurrency paths.",
    );
    expect(screen.queryByText("Input needed")).toBeNull();
    expect(screen.queryByText("Gate")).toBeNull();
    expect(screen.queryByRole("region", { name: "Race detector" })).toBeNull();
    expect(screen.queryByText("race-preview")).toBeNull();
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

  it("folds settled answers into Codex's compact question disclosure", () => {
    const { container } = render(
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

    const disclosure = container.querySelector<HTMLElement>(
      '[data-slot="agent-activity-disclosure"]',
    );
    expect(disclosure).not.toBeNull();
    expect(disclosure!.getAttribute("data-shell")).toBe("line");

    const trigger = screen.getByRole("button", { name: "Asked 1 question" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("Describe the constraints for this change.")).toBeNull();
    expect(screen.queryByText("First constraint\nSecond constraint")).toBeNull();

    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Describe the constraints for this change.")).toBeTruthy();
    const answer = screen.getByText(/First constraint\s+Second constraint/);
    expect(answer.className).toContain("whitespace-pre-wrap");
  });

  it("presents multiple questions one at a time and advances without losing the draft", () => {
    render(
      <QuestionCard
        status="requires-action"
        runId="run-1"
        itemId="question-7"
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
          {
            type: "text",
            header: "Context",
            prompt: "Describe the constraints.",
          },
        ]}
      />,
    );

    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(screen.queryByText("Describe the constraints.")).toBeNull();
    fireEvent.click(screen.getByRole("radio", { name: "Race detector" }));

    expect(screen.getByText("2 of 2")).toBeTruthy();
    expect(screen.getByText("Describe the constraints.")).toBeTruthy();
    expect(screen.queryByText("Which gate should run next?")).toBeNull();
    expect(screen.getByRole("button", { name: "Skip" })).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "Describe the constraints." }), {
      target: { value: "Keep the boundary exact." },
    });
    expect(screen.getByRole("button", { name: "Next" })).toBeTruthy();
  });

  it("keeps an unmarked Chinese-IME commit Enter inside the freeform answer", () => {
    render(
      <QuestionCard
        status="requires-action"
        runId="run-1"
        itemId="question-ime"
        questions={[
          {
            type: "text",
            header: "Context",
            prompt: "Describe the constraint.",
          },
        ]}
      />,
    );

    const answer = screen.getByRole("textbox", { name: "Describe the constraint." });
    fireEvent.compositionStart(answer, { data: "english" });
    fireEvent.change(answer, { target: { value: "中文 english" } });
    fireEvent.compositionEnd(answer, { data: "english" });
    fireEvent.keyDown(answer, { key: "Enter", keyCode: 13, isComposing: false });

    expect(submitQuestion).not.toHaveBeenCalled();

    fireEvent.keyDown(answer, { key: "Enter", keyCode: 13, isComposing: false });
    expect(submitQuestion).toHaveBeenCalledWith([["中文 english"]]);
  });
});
