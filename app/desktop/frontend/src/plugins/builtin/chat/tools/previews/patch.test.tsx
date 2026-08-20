import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { ApplyPatchPreview } from "./patch";

function patchTool(result: string | undefined, status: ToolCall["status"] = "ok"): ToolCall {
  return {
    id: "patch_1",
    runId: "run_1",
    name: "apply_patch",
    fn: "apply_patch",
    args: "",
    result,
    status,
  };
}

describe("ApplyPatchPreview", () => {
  it("renders only the exact call receipt as quiet file-change rows", () => {
    const { container } = render(
      <ApplyPatchPreview
        tool={patchTool(
          '{"changes":[{"path":"src/new.ts","status":"added"},{"path":"src/current.ts","status":"moved","from":"src/old.ts"}]}',
        )}
      />,
    );

    expect(screen.getByText("Created")).toBeTruthy();
    expect(screen.getByText("Moved")).toBeTruthy();
    expect(screen.getByTitle("src/new.ts")).toBeTruthy();
    expect(screen.getByTitle("src/old.ts")).toBeTruthy();
    expect(screen.getByTitle("src/current.ts")).toBeTruthy();
    expect(container.querySelectorAll("[data-patch-change]")).toHaveLength(2);
    expect(container.querySelector("[class*='diff-added']")).toBeNull();
  });

  it("keeps running and completed empty receipts distinct", () => {
    const { rerender } = render(<ApplyPatchPreview tool={patchTool(undefined, "running")} />);
    expect(screen.getByText("Running…")).toBeTruthy();

    rerender(<ApplyPatchPreview tool={patchTool('{"changes":[]}', "ok")} />);
    expect(screen.getByText("No changes to show")).toBeTruthy();
  });

  it("opens the existing full diff owner only through the explicit footer action", () => {
    const onOpenView = vi.fn();
    render(
      <ApplyPatchPreview
        tool={patchTool('{"changes":[{"path":"src/a.ts","status":"modified"}]}')}
        onOpenView={onOpenView}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /open full diff/i }));
    expect(onOpenView).toHaveBeenCalledOnce();
  });
});
