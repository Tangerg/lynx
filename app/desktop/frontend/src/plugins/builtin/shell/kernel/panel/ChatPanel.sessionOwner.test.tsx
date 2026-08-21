import { cleanup, render, screen } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceViewSpec } from "@/plugins/sdk";
import { ChatPanel } from "./ChatPanel";

const model = vi.hoisted(() => ({
  sessionId: "ses_first",
  activeMainView: "stateful" as string | null,
  dock: { open: false, viewIds: [] as string[], activeViewId: null as string | null },
  isLoading: false,
  views: [] as WorkspaceViewSpec[],
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSession: () => null,
  useActiveSessionId: () => model.sessionId,
  useAgentSessions: () => ({ isLoading: model.isLoading }),
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useIsCurrentRootRunning: () => false,
}));

vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  closeWorkspaceDockView: vi.fn(),
  closeWorkspaceView: vi.fn(),
  collapseWorkspaceDock: vi.fn(),
  openWorkspaceViewInDock: vi.fn(),
  selectWorkspaceDockView: vi.fn(),
  showWorkspaceDock: vi.fn(),
  useActiveWorkspaceViewId: () => model.activeMainView,
  useWorkspaceDock: () => model.dock,
}));

vi.mock("@/plugins/builtin/workspace/public/contextDockCatalog", () => ({
  useContextDockCatalog: () => [],
}));

vi.mock("@/plugins/builtin/workspace/public/sidebarDrawer", () => ({
  useDockWidth: () => ({ width: 420, setWidth: vi.fn() }),
}));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  useWorkspaceViews: () => model.views,
}));

vi.mock("@/plugins/host/PluginBoundary", () => ({
  PluginBoundary: ({ children }: { children: ReactNode }) => children,
}));

vi.mock("@/plugins/host/Slot", () => ({ Slot: () => null }));
vi.mock("./ChatStream", () => ({ ChatStream: () => null }));
vi.mock("./HeaderDiffStat", () => ({ HeaderDiffStat: () => null }));

let nextInstance = 0;

function StatefulWorkspaceView() {
  const [instance] = useState(() => ++nextInstance);
  return <span>workspace-instance:{instance}</span>;
}

beforeEach(() => {
  model.isLoading = false;
});

afterEach(() => cleanup());

describe("ChatPanel Session-owned workspace view state", () => {
  it("retires a promoted workspace view when the exact Session changes", () => {
    nextInstance = 0;
    model.sessionId = "ses_first";
    model.activeMainView = "stateful";
    model.dock = { open: false, viewIds: [], activeViewId: null };
    model.views = [
      {
        id: "stateful",
        title: "stateful",
        icon: "tool",
        component: StatefulWorkspaceView,
      },
    ];
    const view = render(<ChatPanel onSend={() => true} />);
    expect(screen.getByText("workspace-instance:1")).toBeTruthy();

    model.sessionId = "ses_second";
    view.rerender(<ChatPanel onSend={() => true} />);

    expect(screen.getByText("workspace-instance:2")).toBeTruthy();
  });

  it("keeps the dock on the same Session ownership boundary", () => {
    nextInstance = 0;
    model.sessionId = "ses_first";
    model.activeMainView = null;
    model.dock = { open: true, viewIds: ["stateful"], activeViewId: "stateful" };
    model.views = [
      {
        id: "stateful",
        title: "stateful",
        icon: "tool",
        component: StatefulWorkspaceView,
      },
    ];
    const view = render(<ChatPanel onSend={() => true} />);
    expect(screen.getByText("workspace-instance:1")).toBeTruthy();

    model.sessionId = "ses_second";
    view.rerender(<ChatPanel onSend={() => true} />);

    expect(screen.getByText("workspace-instance:2")).toBeTruthy();
  });

  it("does not mount unowned dock material without an active Session", () => {
    nextInstance = 0;
    model.sessionId = "";
    model.activeMainView = null;
    model.dock = { open: true, viewIds: ["stateful"], activeViewId: "stateful" };
    model.views = [
      {
        id: "stateful",
        title: "stateful",
        icon: "tool",
        component: StatefulWorkspaceView,
      },
    ];

    const view = render(<ChatPanel onSend={() => true} />);

    expect(screen.queryByText(/workspace-instance:/)).toBeNull();
    expect(view.container.querySelector('[data-dock="open"]')).toBeNull();
  });

  it("reconciles dock availability after the loading placeholder yields to the shell", () => {
    model.sessionId = "ses_first";
    model.activeMainView = null;
    model.dock = { open: false, viewIds: [], activeViewId: null };
    model.views = [];
    model.isLoading = true;

    const view = render(<ChatPanel onSend={() => true} />);
    expect(view.container.firstChild).toBeNull();

    model.isLoading = false;
    view.rerender(<ChatPanel onSend={() => true} />);

    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: "Widen the window to open the right workspace",
      }).disabled,
    ).toBe(true);
  });
});
