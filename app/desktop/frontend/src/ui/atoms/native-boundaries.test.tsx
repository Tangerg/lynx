import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ColorPickerInput } from "./color-picker-input";
import { ExternalLink } from "./external-link";
import { HiddenFileInput } from "./hidden-file-input";

describe("native design-system boundaries", () => {
  it("owns browser-native input types inside atoms", () => {
    render(
      <>
        <HiddenFileInput aria-label="Attach" accept="image/*" />
        <ColorPickerInput aria-label="Accent" />
      </>,
    );

    expect(screen.getByLabelText("Attach").getAttribute("type")).toBe("file");
    expect(screen.getByLabelText("Accent").getAttribute("type")).toBe("color");
  });

  it("owns safe external-link policy", () => {
    render(<ExternalLink href="https://example.com">Source</ExternalLink>);

    const link = screen.getByRole("link", { name: "Source" });
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
  });
});
