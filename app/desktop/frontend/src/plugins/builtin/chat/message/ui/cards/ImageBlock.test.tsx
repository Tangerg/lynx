import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MESSAGE_CONTENT_CLASS } from "../messageContent";
import { MarkdownMessage } from "../markdown/MarkdownMessage";
import { ImageBlock } from "./ImageBlock";

const mocks = vi.hoisted(() => ({
  saveImage: vi.fn<(source: string) => Promise<boolean>>(),
}));

vi.mock("@/main/container", () => ({
  getContainer: () => ({ desktop: { saveImage: mocks.saveImage } }),
}));

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
  beforeEach(() => {
    mocks.saveImage.mockReset();
    mocks.saveImage.mockResolvedValue(true);
  });

  it("saves the active gallery image through the packaged desktop owner", async () => {
    render(
      <div className={MESSAGE_CONTENT_CLASS}>
        <ImageBlock mime="image/png" data={FIRST_PNG} />
        <ImageBlock mime="image/gif" data={SECOND_GIF} />
      </div>,
    );

    await openPreview(screen.getAllByRole("button", { name: "View attached image" })[0]!);
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));
    fireEvent.click(screen.getByRole("button", { name: "Download image" }));

    await waitFor(() => {
      expect(mocks.saveImage).toHaveBeenCalledWith(`data:image/gif;base64,${SECOND_GIF}`);
    });
  });

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
