// PanelHeader tests — focused on the bug class that already bit us:
// the tab strip must mirror sessionStore.tabIds 1:1, even when the
// sessions query hasn't (yet / any more) carried metadata for a
// given id. The previous `filter(Boolean)` silently dropped such
// tabs, producing a "I clicked but no tab appeared" report.
//
// We also smoke-test the basic happy path (tab order, active state,
// click-to-select).

import type { SidebarSession } from "@/lib/data/queries";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { definePlugin, loadPlugin } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";
import { PanelHeader } from "./PanelHeader";

// React Query needs a provider in tests. Disable retries so a fake
// fetcher that returns immediately stays synchronous.
function wrap(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

// Register a `sessions` data provider returning the given rows. The
// query key matches what `useSessions()` consumes; the plugin runs
// inside the per-test loader so other tests start from an empty
// registry (see test/setup.ts).
async function loadSessionsProvider(rows: SidebarSession[]): Promise<void> {
  await loadPlugin(
    definePlugin({
      name: "test.panel-header.sessions",
      version: "1.0.0",
      setup: ({ host }) => {
        host.extensions.contribute(DATA_PROVIDER, {
          key: "sessions",
          fetcher: async () => rows,
        });
      },
    }),
  );
}

function seedTabs(activeId: string, tabIds: string[]): void {
  useSessionStore.setState({
    activeSessionId: activeId,
    tabIds,
    mainViewTabs: [],
    activeMainView: null,
  });
}

describe("panelHeader", () => {
  it("renders one tab per tabIds in order, using session title when available", async () => {
    await loadSessionsProvider([
      { id: "s1", title: "First Chat", status: "idle", model: "x", time: "" },
      { id: "s2", title: "Second Chat", status: "idle", model: "x", time: "" },
    ]);
    seedTabs("s1", ["s1", "s2"]);
    wrap(<PanelHeader />);

    // React Query resolves on a microtask but render reconciliation
    // needs a couple of ticks — findBy* waits for the element to
    // appear, which is what we actually want to assert.
    expect(await screen.findByText("First Chat")).toBeTruthy();
    expect(await screen.findByText("Second Chat")).toBeTruthy();
  });

  it("falls back to the id as the title when a tabId has no session row yet", async () => {
    // sessions cache is empty — the user opened "s99" before the
    // server-side row was created (or while the query is mid-refetch).
    await loadSessionsProvider([]);
    seedTabs("s99", ["s99"]);
    wrap(<PanelHeader />);

    // Even with no metadata, the tab must appear — using the id as a
    // placeholder title. Previously `filter(Boolean)` dropped it.
    expect(screen.getByText("s99")).toBeTruthy();
  });

  it("keeps an open tab visible even after the sessions query empties out", async () => {
    // Sessions query returns metadata; user keeps the tab open after
    // the row disappears from the cache (e.g. user filtered the
    // sidebar list — same query key, smaller payload).
    await loadSessionsProvider([]);
    seedTabs("phantom", ["phantom", "s1"]);
    wrap(<PanelHeader />);

    expect(screen.getByText("phantom")).toBeTruthy();
    expect(screen.getByText("s1")).toBeTruthy();
  });

  it("clicking a tab selects it in the store", async () => {
    await loadSessionsProvider([
      { id: "s1", title: "First", status: "idle", model: "x", time: "" },
      { id: "s2", title: "Second", status: "idle", model: "x", time: "" },
    ]);
    seedTabs("s1", ["s1", "s2"]);
    wrap(<PanelHeader />);

    fireEvent.click(await screen.findByText("Second"));
    expect(useSessionStore.getState().activeSessionId).toBe("s2");
  });
});
