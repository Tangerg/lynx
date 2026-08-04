import { describe, expect, it } from "vitest";
import { foldExchanges, type AnchoredTurn } from "./transcriptAnchors";

const turn = (id: string, role: string | null, top: number): AnchoredTurn => ({ id, role, top });

describe("foldExchanges", () => {
  // The bug this exists for: the rail's unit is the exchange and the DOM's is the
  // message, so scrolling from a question into its own answer put an id under the
  // reading line that no mark carried, and the highlight went out.
  it("keeps a question and its answer as one exchange, named by the question", () => {
    expect(
      foldExchanges([
        turn("u1", "user", 0),
        turn("a1", "assistant", 100),
        turn("u2", "user", 400),
        turn("a2", "assistant", 480),
      ]).map((exchange) => exchange.id),
    ).toEqual(["u1", "u2"]);
  });

  // A compaction note is a system message between two exchanges. It belongs to the
  // one it interrupts, not to a mark of its own.
  it("attributes anything that is not a question to the exchange it follows", () => {
    expect(
      foldExchanges([
        turn("u1", "user", 0),
        turn("a1", "assistant", 100),
        turn("s1", "system", 300),
        turn("a2", "assistant", 340),
        turn("u2", "user", 700),
      ]).map((exchange) => exchange.id),
    ).toEqual(["u1", "u2"]);
  });

  // A restored session can open on an assistant turn. Its height has to belong to
  // something, or the tallest-exchange scale is computed from a gap.
  it("opens an exchange for a transcript that does not start with a question", () => {
    expect(
      foldExchanges([
        turn("a0", "assistant", 0),
        turn("u1", "user", 200),
        turn("a1", "assistant", 260),
      ]).map((exchange) => exchange.id),
    ).toEqual(["a0", "u1"]);
  });

  it("has nothing to fold when the transcript is empty", () => {
    expect(foldExchanges([])).toEqual([]);
  });
});
