import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import { useSessionSearchStore } from "../../sessionSearchStore";
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
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  client.setQueryData([AGENT_SESSIONS_KEY], []);
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

beforeEach(() => selectAgentSession.mockClear());
afterEach(() => useSessionSearchStore.setState({ open: false }));

// A surface that renders and does NOTHING is the failure this file exists for, and
// it shipped once: the palette's rows lost the props that made them rows, so the
// list looked right and nothing in it could be run by mouse or keyboard. Every
// assertion here is about a row doing its job.
describe("going to a session", () => {
  it("opens the session a row is clicked on, and closes", () => {
    wrap(open());

    fireEvent.click(screen.getByText("Fix the retry loop"));

    expect(selectAgentSession).toHaveBeenCalledWith("ses_a");
    expect(useSessionSearchStore.getState().open).toBe(false);
  });

  it("opens the highlighted session on Enter, after the arrows move it", () => {
    wrap(open());
    const panel = screen.getByRole("listbox");

    // Newest first, so the highlight starts on "Rename the dock".
    fireEvent.keyDown(panel, { key: "ArrowDown" });
    fireEvent.keyDown(panel, { key: "Enter" });

    expect(selectAgentSession).toHaveBeenCalledWith("ses_a");
  });

  it("marks exactly one row, and moves it with the query", () => {
    wrap(open());

    const marked = () =>
      screen.getAllByRole("option").filter((row) => row.getAttribute("aria-selected") === "true");
    expect(marked()).toHaveLength(1);
    expect(marked()[0]?.textContent).toContain("Rename the dock");

    fireEvent.change(screen.getByRole("textbox"), { target: { value: "retry" } });

    expect(screen.getAllByRole("option")).toHaveLength(1);
    expect(marked()[0]?.textContent).toContain("Fix the retry loop");
  });

  it("says so when nothing matches, instead of showing an empty box", () => {
    wrap(open());

    fireEvent.change(screen.getByRole("textbox"), { target: { value: "nothing here" } });

    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(screen.getByText("No session matches")).toBeTruthy();
  });
});
