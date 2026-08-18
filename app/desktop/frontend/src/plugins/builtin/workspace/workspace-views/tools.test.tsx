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
  it("retires Runtime result material without discarding the user's diagnostic draft", async () => {
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
    const argumentsInput = screen.getByRole("textbox", { name: "Arguments (JSON object)" });
    fireEvent.change(argumentsInput, { target: { value: '{"path":"README.md"}' } });
    fireEvent.click(screen.getByRole("button", { name: "Run diagnostic" }));
    await screen.findByText(/"matches": 2/);

    await act(async () => owner!.replaceRuntimeGeneration());

    expect(screen.queryByText(/"matches": 2/)).toBeNull();
    expect(screen.getByRole("button", { name: "Close grep diagnostic" })).toBeTruthy();
    expect(
      (screen.getByRole("textbox", { name: "Arguments (JSON object)" }) as HTMLTextAreaElement)
        .value,
    ).toBe('{"path":"README.md"}');
  });
});
