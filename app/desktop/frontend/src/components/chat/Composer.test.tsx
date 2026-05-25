import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { definePlugin, loadPlugin } from "@/plugins/sdk";
import { Composer } from "./Composer";

// Composer relies on a built-in composer-keymap registration to bind
// Enter → submit. Set up a tiny in-test plugin that mirrors it.
async function withEnterKeymap() {
  await loadPlugin(
    definePlugin({
      name: "test.composer-keymap",
      version: "1.0.0",
      setup: ({ host }) => {
        host.composer.registerKeyBinding({
          key: "Enter",
          description: "submit",
          handler: ({ submit }) => {
            submit();
            return true;
          },
        });
      },
    }),
  );
}

const baseProps = {
  attachments: [],
  onRemoveAttachment: () => {},
  mode: "agent" as const,
  onModeChange: () => {},
};

describe("composer", () => {
  it("calls onChange as the user types", () => {
    const onChange = vi.fn();
    render(<Composer {...baseProps} value="" onChange={onChange} onSend={() => {}} />);
    const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "hi" } });
    expect(onChange).toHaveBeenCalledWith("hi");
  });

  it("submits non-empty text on Enter when a binding maps Enter → submit", async () => {
    await withEnterKeymap();
    const onSend = vi.fn();
    const onChange = vi.fn();
    render(<Composer {...baseProps} value="hello world" onChange={onChange} onSend={onSend} />);
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
    expect(onSend).toHaveBeenCalledWith("hello world");
  });

  it("does not submit when the textarea is empty / whitespace only", async () => {
    await withEnterKeymap();
    const onSend = vi.fn();
    render(<Composer {...baseProps} value="   " onChange={() => {}} onSend={onSend} />);
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
    expect(onSend).not.toHaveBeenCalled();
  });

  it("renders attachment chips when attachments are provided", () => {
    render(
      <Composer
        {...baseProps}
        value=""
        onChange={() => {}}
        onSend={() => {}}
        attachments={[{ label: "src/app.ts" }, { label: "src/auth.ts" }]}
      />,
    );
    expect(screen.getByText("src/app.ts")).toBeTruthy();
    expect(screen.getByText("src/auth.ts")).toBeTruthy();
  });
});
