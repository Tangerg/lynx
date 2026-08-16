import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { WorkspaceCommandActivity } from "@/plugins/builtin/workspace/application/toolActivity";
import { CommandLog } from "./CommandLog";

const command = (id: string, text: string): WorkspaceCommandActivity => ({
  id,
  command: text,
  status: "succeeded",
  output: "",
});

describe("CommandLog selection", () => {
  it("marks only the exact tool item selected by the conversation", () => {
    const view = render(
      <CommandLog
        commands={[command("cmd-old", "npm test"), command("cmd-new", "npm run build")]}
        selectedCommandId="cmd-old"
      />,
    );

    expect(view.container.querySelectorAll("[data-command-selected]")).toHaveLength(1);
    expect(
      screen
        .getByText("npm test")
        .closest("[data-command-id]")
        ?.getAttribute("data-command-selected"),
    ).toBe("");
    expect(
      screen
        .getByText("npm run build")
        .closest("[data-command-id]")
        ?.getAttribute("data-command-selected"),
    ).toBeNull();
  });
});
