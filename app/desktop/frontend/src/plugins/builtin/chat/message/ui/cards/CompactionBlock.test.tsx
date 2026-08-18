import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CompactionBlock } from "./CompactionBlock";

describe("CompactionBlock", () => {
  it("presents the automatic context boundary without implementation counts", () => {
    render(<CompactionBlock droppedMessages={8} />);

    expect(screen.getByText("Context automatically compacted")).toBeTruthy();
    expect(screen.queryByText(/8/)).toBeNull();
  });
});
