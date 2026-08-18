import { render, screen } from "@testing-library/react";
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
            options: [
              { label: "Race detector", description: "Exercise concurrency paths." },
              { label: "Visual suite", description: "Verify visual states." },
            ],
          },
        ]}
      />,
    );

    expect(screen.getByRole("radiogroup", { name: "Which gate should run next?" })).toBeVisible();
    expect(screen.getByRole("radio", { name: /Race detector/ })).toHaveAttribute(
      "aria-checked",
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
            prompt: "Which checks should run?",
            multiple: true,
            options: [{ label: "Race detector" }, { label: "Visual suite" }],
          },
        ]}
      />,
    );

    expect(screen.getAllByRole("checkbox")).toHaveLength(2);
    expect(screen.getByRole("checkbox", { name: "Race detector" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
  });
});
