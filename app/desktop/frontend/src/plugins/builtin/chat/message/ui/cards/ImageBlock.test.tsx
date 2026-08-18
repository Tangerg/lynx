import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MESSAGE_CONTENT_CLASS } from "../messageContent";
import { MarkdownMessage } from "../markdown/MarkdownMessage";
import { ImageBlock } from "./ImageBlock";

const FIRST_PNG =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
const SECOND_GIF = "R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==";

async function openPreview(button: HTMLElement): Promise<void> {
  fireEvent.click(button);
  // Base UI assigns initial dialog focus in a microtask after its layout effects.
  // Keep the test alive through that documented open lifecycle instead of leaving
  // the focus job pending for Testing Library's automatic unmount.
  await act(async () => Promise.resolve());
}

describe("ImageBlock", () => {
  it("navigates protocol images inside one message preview gallery", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );

    await openPreview(screen.getAllByRole("button", { name: "View attached image" })[0]!);
    expect(screen.getByRole("dialog", { name: "View attached image" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));

    expect(
      screen
        .getByRole("dialog", { name: "View attached image" })
        .querySelector("img")
        ?.getAttribute("src"),
    ).toBe(`data:image/gif;base64,${SECOND_GIF}`);
  });

  it("keeps protocol and Markdown images in the same message gallery", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <MarkdownMessage text={`![Inline](data:image/gif;base64,${SECOND_GIF})`} reveal="instant" />
      </div>,
    );

    await openPreview(screen.getByRole("button", { name: "View attached image" }));
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));

    expect(screen.getByRole("dialog", { name: "Inline" })).toBeTruthy();
  });

  it("does not include images from a nested message body", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <div className={MESSAGE_CONTENT_CLASS}>
          <ImageBlock mime="image/gif" data={SECOND_GIF} />
        </div>
      </div>,
    );

    await openPreview(screen.getAllByRole("button", { name: "View attached image" })[0]!);

    expect(screen.queryByRole("button", { name: "Next image" })).toBeNull();
  });
});
