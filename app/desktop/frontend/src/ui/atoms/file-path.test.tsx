import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { FilePath } from "./file-path";

// The visible outcome — which end clips — is CSS, so what is pinned here is the
// split that makes it possible: the filename in its own unshrinkable element, the
// directory in the elastic one.
describe("FilePath", () => {
  it("pins the filename and lets the directory take the squeeze", () => {
    const { container } = render(<FilePath path="src/plugins/builtin/chat/Composer.tsx" />);
    const parts = [...(container.firstElementChild?.children ?? [])];

    expect(parts.map((p) => p.textContent)).toEqual([
      "src/plugins/builtin/chat",
      "/",
      "Composer.tsx",
    ]);
    // rtl on the directory alone is what moves the ellipsis to its left edge; the
    // separator is a sibling so the bidi run cannot take it along.
    expect(parts[0]?.getAttribute("dir")).toBe("rtl");
    expect(parts[0]?.className).toContain("truncate");
    // The filename gives way only after the directory has nothing left to give, and
    // then with an ellipsis: pinned outright it became the row's min-content and pushed
    // the whole row past a column narrower than the name.
    expect(parts[2]?.className).toContain("shrink");
    expect(parts[2]?.className).toContain("truncate");
  });

  it("isolates the directory's own text direction inside the rtl clip", () => {
    const { container } = render(<FilePath path="/Users/me/app/tools/preview.ts" />);
    const clip = container.querySelector("[dir=rtl]");
    // The whole directory sits in ONE ltr isolate. Without it the leading `/` of an
    // absolute path takes the surrounding rtl direction and is reordered to the far
    // side of the run, next to the separator — a second slash appears out of nowhere.
    expect(clip?.children).toHaveLength(1);
    expect(clip?.firstElementChild?.getAttribute("dir")).toBe("ltr");
    expect(clip?.firstElementChild?.textContent).toBe("/Users/me/app/tools");
  });

  it("renders a bare filename as just the filename", () => {
    const { container } = render(<FilePath path="go.mod" />);
    expect(container.textContent).toBe("go.mod");
    expect(container.querySelector("[dir=rtl]")).toBeNull();
  });

  it("keeps an absolute path's leading slash out of the filename", () => {
    const { container } = render(<FilePath path="/etc/hosts" />);
    expect(container.textContent).toBe("/etc/hosts");
  });

  it("keeps the slash of a root-level path, which has no directory to hold it", () => {
    const { container } = render(<FilePath path="/LICENSE" />);
    expect(container.textContent).toBe("/LICENSE");
  });

  it("carries the whole path as the title, since part of it may be clipped", () => {
    const { container } = render(<FilePath path="a/b/c.ts" />);
    expect(container.firstElementChild?.getAttribute("title")).toBe("a/b/c.ts");
  });
});
