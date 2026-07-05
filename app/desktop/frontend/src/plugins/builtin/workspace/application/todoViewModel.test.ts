import { describe, expect, it } from "vitest";
import {
  workspaceTodosSubtext,
  workspaceTodosViewModel,
  type WorkspaceTodo,
} from "./todoViewModel";

const todo = (over: Partial<WorkspaceTodo>): WorkspaceTodo => ({
  id: "todo-1",
  text: "Inspect workspace",
  status: "pending",
  ...over,
});

describe("workspaceTodosViewModel", () => {
  it("projects an unavailable checklist", () => {
    expect(workspaceTodosViewModel(false, [todo({ status: "completed" })])).toEqual({
      enabled: false,
      todos: [todo({ status: "completed" })],
      done: 1,
      total: 1,
      state: "unavailable",
    });
  });

  it("projects an empty enabled checklist", () => {
    expect(workspaceTodosViewModel(true, [])).toEqual({
      enabled: true,
      todos: [],
      done: 0,
      total: 0,
      state: "empty",
    });
  });

  it("counts completed todos and keeps source order", () => {
    const first = todo({ id: "todo-1", status: "completed" });
    const second = todo({ id: "todo-2", status: "in_progress" });
    const third = todo({ id: "todo-3", status: "pending" });

    expect(workspaceTodosViewModel(true, [first, second, third])).toEqual({
      enabled: true,
      todos: [first, second, third],
      done: 1,
      total: 3,
      state: "ready",
    });
  });
});

describe("workspaceTodosSubtext", () => {
  it("omits header subtext when there are no todos", () => {
    expect(workspaceTodosSubtext(workspaceTodosViewModel(true, []))).toBeUndefined();
  });

  it("builds completion subtext", () => {
    expect(
      workspaceTodosSubtext(workspaceTodosViewModel(true, [todo({ status: "completed" })])),
    ).toBe("1 of 1 done");
  });
});
