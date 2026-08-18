import { afterEach, describe, expect, it } from "vitest";
import { markdownSelectionPayload } from "./markdownSelectionCopy";

function selectContents(element: Element): Selection {
  const range = document.createRange();
  range.selectNodeContents(element);
  const selection = document.getSelection()!;
  selection.removeAllRanges();
  selection.addRange(range);
  return selection;
}

afterEach(() => {
  document.getSelection()?.removeAllRanges();
  document.body.replaceChildren();
});

describe("markdown selection copy", () => {
  it("copies a selected code artifact from its authoritative source payload", () => {
    const root = document.createElement("div");
    root.innerHTML =
      '<div data-markdown-copy="code-block" data-markdown-copy-text="const value = 1 &lt; 2"><header data-markdown-copy="exclude">typescript</header><pre>painted tokens</pre></div>';
    document.body.append(root);
    const block = root.firstElementChild!;

    const payload = markdownSelectionPayload(root, selectContents(block));

    expect(payload?.plainText).toBe("const value = 1 < 2");
    expect(payload?.htmlText).toBe('<pre dir="ltr"><code>const value = 1 &lt; 2</code></pre>');
  });

  it("replaces artifacts and removes action chrome in a mixed selection", () => {
    const root = document.createElement("div");
    root.innerHTML = [
      "<p>Before</p>",
      '<div data-markdown-copy="code-block" data-markdown-copy-text="npm test">visual graph</div>',
      '<button data-markdown-copy="exclude">Copy</button>',
      "<p>After</p>",
    ].join("");
    document.body.append(root);

    const payload = markdownSelectionPayload(root, selectContents(root));

    expect(payload?.plainText).toBe("Before\nnpm test\nAfter");
    expect(payload?.htmlText).toContain('<pre dir="ltr"><code>npm test</code></pre>');
    expect(payload?.htmlText).not.toContain("Copy");
    expect(payload?.htmlText).not.toContain("visual graph");
  });
});
