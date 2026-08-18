import { afterEach, describe, expect, it, vi } from "vitest";
import { copyRichText } from "./clipboard";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("copyRichText", () => {
  it("writes Markdown and rendered HTML together when the Clipboard API supports it", async () => {
    const write = vi.fn().mockResolvedValue(undefined);
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { write, writeText } });

    const clipboardItem = vi.fn<(formats: Record<string, Blob>) => void>();
    vi.stubGlobal(
      "ClipboardItem",
      class ClipboardItemStub {
        constructor(formats: Record<string, Blob>) {
          clipboardItem(formats);
        }
      },
    );

    await expect(
      copyRichText({
        plainText: "| Name | Count |\n|---|---:|\n| alpha | 12 |",
        htmlText: "<table><tbody><tr><td>alpha</td><td>12</td></tr></tbody></table>",
      }),
    ).resolves.toBe(true);

    expect(write).toHaveBeenCalledOnce();
    expect(writeText).not.toHaveBeenCalled();
    const formats = clipboardItem.mock.calls[0]![0];
    await expect(formats["text/plain"]!.text()).resolves.toContain("| Name | Count |");
    await expect(formats["text/html"]!.text()).resolves.toContain("<table>");
  });
});
