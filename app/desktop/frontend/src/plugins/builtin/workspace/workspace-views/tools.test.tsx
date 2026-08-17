import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DiagnosticToolOwner } from "../application/diagnosticTool";
import type { DiagnosticToolGateway } from "../application/ports/diagnosticToolGateway";
import { DiagnosticToolRow } from "./tools";

let owner: DiagnosticToolOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
});

describe("DiagnosticToolRow", () => {
  it("retires completed result material at an in-place Runtime generation change", async () => {
    owner = DiagnosticToolOwner.install({
      invoke: vi.fn().mockResolvedValue({ matches: 2 }),
    } as DiagnosticToolGateway);
    render(
      <DiagnosticToolRow
        tool={{
          id: "grep",
          name: "grep",
          description: "Search files",
          icon: "search",
          parameters: { type: "object" },
        }}
        cwd="/work/alpha"
        enabled
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Configure grep diagnostic" }));
    fireEvent.click(screen.getByRole("button", { name: "Run diagnostic" }));
    await screen.findByText(/"matches": 2/);

    await act(async () => owner!.replaceRuntimeGeneration());

    expect(screen.queryByText(/"matches": 2/)).toBeNull();
    expect(screen.getByRole("button", { name: "Configure grep diagnostic" })).toBeTruthy();
  });
});
