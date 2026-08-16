import { render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { SessionOwnedDock } from "./SessionOwnedDock";

let nextIdentity = 0;

function StatefulView() {
  const [identity] = useState(() => ++nextIdentity);
  return <span>instance:{identity}</span>;
}

describe("SessionOwnedDock", () => {
  it("retires local view state when the exact Session owner changes", () => {
    nextIdentity = 0;
    const view = render(
      <SessionOwnedDock sessionId="s1">
        <StatefulView />
      </SessionOwnedDock>,
    );
    expect(screen.getByText("instance:1")).toBeTruthy();

    view.rerender(
      <SessionOwnedDock sessionId="s2">
        <StatefulView />
      </SessionOwnedDock>,
    );
    expect(screen.getByText("instance:2")).toBeTruthy();

    view.rerender(
      <SessionOwnedDock sessionId="s2">
        <StatefulView />
      </SessionOwnedDock>,
    );
    expect(screen.getByText("instance:2")).toBeTruthy();
  });
});
