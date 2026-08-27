import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import { drainBrowserTasks } from "@/test/browserTasks";
import { useSessionSearchStore } from "../application/sessionSearchState";
import { SessionSearch } from "./SessionSearch";

const selectAgentSession = vi.hoisted(() => vi.fn());
vi.mock("@/plugins/builtin/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/builtin/agent/public/session")>()),
  selectAgentSession,
  useAgentSessions: () => ({
    data: [
      {
        id: "ses_a",
        revision: 1,
        title: "Fix the retry loop",
        status: "idle",
        time: "2026-08-01T10:00:00.000Z",
      },
      {
        id: "ses_b",
        revision: 1,
        title: "Rename the dock",
        status: "idle",
        time: "2026-08-03T10:00:00.000Z",
      },
    ],
  }),
}));

function open(): ReactElement {
  useSessionSearchStore.setState({ open: true });
  return <SessionSearch />;
}

function wrap(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
    },
  });
  client.setQueryData([AGENT_SESSIONS_KEY], []);
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

const field = () => screen.getByRole("combobox");
const marked = () =>
  screen.getAllByRole("option").filter((row) => row.getAttribute("aria-selected") === "true");

beforeEach(() => selectAgentSession.mockClear());
afterEach(async () => {
  useSessionSearchStore.setState({ open: false });
  cleanup();
  await drainBrowserTasks();
});

// A surface that renders and does NOTHING is the failure this file exists for, and
// it shipped once: the palette's rows lost the props that made them rows, so the
// list looked right and nothing in it could be run by mouse or keyboard.
describe("going to a session", () => {
  it("opens the session a row is clicked on, and closes", () => {
    wrap(open());

    fireEvent.click(screen.getByText("Fix the retry loop"));

    expect(selectAgentSession).toHaveBeenCalledWith("ses_a");
    expect(useSessionSearchStore.getState().open).toBe(false);
  });

  it("opens the highlighted session on Enter, after the arrows move it", () => {
    wrap(open());

    // Newest first, so the highlight starts on "Rename the dock".
    fireEvent.keyDown(field(), { key: "ArrowDown" });
    fireEvent.keyDown(field(), { key: "Enter" });

    expect(selectAgentSession).toHaveBeenCalledWith("ses_a");
  });

  it("marks exactly one row, and moves it with the query", () => {
    wrap(open());

    expect(marked()).toHaveLength(1);
    expect(marked()[0]?.textContent).toContain("Rename the dock");

    fireEvent.change(field(), { target: { value: "retry" } });

    expect(screen.getAllByRole("option")).toHaveLength(1);
    expect(marked()[0]?.textContent).toContain("Fix the retry loop");
  });

  it("says so when nothing matches, instead of showing an empty box", () => {
    wrap(open());

    fireEvent.change(field(), { target: { value: "nothing here" } });

    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(screen.getByText("No session matches")).toBeTruthy();
  });

  it("starts from a fresh query and highlight after an external close", () => {
    wrap(open());
    fireEvent.change(field(), { target: { value: "retry" } });
    expect(screen.getAllByRole("option")).toHaveLength(1);

    act(() => useSessionSearchStore.getState().setOpen(false));
    act(() => useSessionSearchStore.getState().setOpen(true));

    expect((field() as HTMLInputElement).value).toBe("");
    expect(screen.getAllByRole("option")).toHaveLength(2);
    expect(marked()[0]?.textContent).toContain("Rename the dock");
  });
});

// Focus stays in the field, so the field is the only thing that can say which row
// the keyboard is on, and the rows must not be tab stops. Both were unmet and both
// are invisible to anyone driving this with a mouse.
describe("reaching the list without a pointer", () => {
  it("announces the highlighted row from the field", () => {
    wrap(open());

    expect(field().getAttribute("aria-activedescendant")).toBe(marked()[0]?.id);

    fireEvent.keyDown(field(), { key: "ArrowDown" });

    expect(marked()[0]?.textContent).toContain("Fix the retry loop");
    expect(field().getAttribute("aria-activedescendant")).toBe(marked()[0]?.id);
  });

  it("names a list for the field to point at, and keeps rows out of the tab order", () => {
    wrap(open());

    expect(field().getAttribute("aria-controls")).toBe(screen.getByRole("listbox").id);
    for (const row of screen.getAllByRole("option")) expect(row.tabIndex).toBe(-1);
  });

  it("stops announcing a row when none is left to announce", () => {
    wrap(open());

    fireEvent.change(field(), { target: { value: "nothing here" } });

    expect(field().getAttribute("aria-activedescendant")).toBe(null);
  });

  it("leaves navigation and acceptance keys with an active IME", () => {
    wrap(open());
    const initial = marked()[0]?.textContent;

    fireEvent.keyDown(field(), { key: "ArrowDown", isComposing: true });
    fireEvent.keyDown(field(), { key: "Enter", isComposing: true });

    expect(marked()[0]?.textContent).toBe(initial);
    expect(selectAgentSession).not.toHaveBeenCalled();
  });
});
