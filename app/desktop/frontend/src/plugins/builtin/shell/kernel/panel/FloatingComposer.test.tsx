import { createRef, type ComponentProps, type ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeServiceSnapshot } from "@/plugins/builtin/runtime/public/serviceStatus";
import { FloatingComposer } from "./FloatingComposer";

const runtime = vi.hoisted(() => ({
  snapshot: {
    phase: "reconnecting",
    observation: null,
    failure: null,
  } as RuntimeServiceSnapshot,
}));

const navigation = vi.hoisted(() => ({
  openSettings: vi.fn(),
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeServiceStatus: () => runtime.snapshot,
}));

vi.mock("motion/react", () => ({
  AnimatePresence: ({ children }: { children: ReactNode }) => children,
  motion: {
    div: ({
      children,
      initial: _initial,
      animate: _animate,
      exit: _exit,
      transition: _transition,
      ...props
    }: ComponentProps<"div"> & {
      initial?: unknown;
      animate?: unknown;
      exit?: unknown;
      transition?: unknown;
    }) => <div {...props}>{children}</div>,
  },
}));

vi.mock("./JumpToBottomButton", () => ({
  JumpToBottomButton: () => null,
}));

vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  openWorkspaceSettingsPane: navigation.openSettings,
}));

describe("FloatingComposer Runtime connection material", () => {
  beforeEach(() => {
    runtime.snapshot.phase = "reconnecting";
    runtime.snapshot.failure = null;
    navigation.openSettings.mockClear();
  });

  it("explains withdrawn command rights while the exact connection recovers", () => {
    render(
      <FloatingComposer overlayRef={createRef<HTMLDivElement>()}>
        <div>Composer</div>
      </FloatingComposer>,
    );

    expect(screen.getByText("Runtime connection was lost. Reconnecting…")).toBeTruthy();
  });

  it("does not flash a loss notice during cold-start inspection", () => {
    runtime.snapshot.phase = "checking";

    render(
      <FloatingComposer overlayRef={createRef<HTMLDivElement>()}>
        <div>Composer</div>
      </FloatingComposer>,
    );

    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.getByText("Composer")).toBeTruthy();
  });

  it("keeps a failed recovery actionable while automatic retries continue", () => {
    runtime.snapshot.phase = "unavailable";
    runtime.snapshot.failure = { reason: "failed", detail: "connection refused" };

    render(
      <FloatingComposer overlayRef={createRef<HTMLDivElement>()}>
        <div>Composer</div>
      </FloatingComposer>,
    );

    expect(screen.getByText("Runtime is unavailable. Lyra will keep trying.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Connection settings" }));
    expect(navigation.openSettings).toHaveBeenCalledWith("connection");
  });
});
