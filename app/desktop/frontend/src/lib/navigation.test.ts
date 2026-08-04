import { beforeEach, describe, expect, it, vi } from "vitest";

import { configureNavigator, navigator } from "./navigation";
import { createMemoryNavigator } from "./navigation.testkit";

// The contract, exercised through the in-memory implementation. The router-backed
// one is the same shape over the same helpers; what is worth pinning here is the
// behaviour every caller relies on: a patch is a patch, and history is real.
beforeEach(() => {
  configureNavigator(createMemoryNavigator());
});

describe("a location patch", () => {
  it("leaves out means leave alone, null means clear", () => {
    navigator().go({ session: "sess_1", dock: "review", settings: "providers" });
    navigator().go({ settings: null });

    expect(navigator().get()).toEqual({
      session: "sess_1",
      view: null,
      dock: "review",
      settings: null,
    });
  });

  it("notifies subscribers with both sides of the move", () => {
    const seen = vi.fn();
    navigator().subscribe(seen);

    navigator().go({ session: "sess_1" });

    expect(seen).toHaveBeenCalledTimes(1);
    expect(seen.mock.calls[0]?.[0]).toMatchObject({ session: "sess_1" });
    expect(seen.mock.calls[0]?.[1]).toMatchObject({ session: "" });
  });

  it("is not a move when nothing changes", () => {
    navigator().go({ view: "diff" });
    const seen = vi.fn();
    navigator().subscribe(seen);

    navigator().go({ view: "diff" });

    expect(seen).not.toHaveBeenCalled();
  });
});

describe("history", () => {
  it("goes back to where it came from, and forward again", () => {
    navigator().go({ session: "sess_1" });
    navigator().go({ session: "sess_2" });

    navigator().back();
    expect(navigator().get().session).toBe("sess_1");

    navigator().forward();
    expect(navigator().get().session).toBe("sess_2");
  });

  it("stops at the ends instead of falling off them", () => {
    navigator().go({ session: "sess_1" });

    navigator().back();
    navigator().back();
    expect(navigator().get().session).toBe("");

    navigator().forward();
    navigator().forward();
    expect(navigator().get().session).toBe("sess_1");
  });

  it("replace corrects the current entry without adding one to go back to", () => {
    // How a cold start seeds the last session: the app was never anywhere else,
    // so there must be nothing behind it.
    navigator().go({ session: "sess_restored" }, { replace: true });

    navigator().back();

    expect(navigator().get().session).toBe("sess_restored");
  });

  it("forward is dropped once a new move is made from a back entry", () => {
    navigator().go({ view: "a" });
    navigator().go({ view: "b" });
    navigator().back();

    navigator().go({ view: "c" });
    navigator().forward();

    expect(navigator().get().view).toBe("c");
  });
});
