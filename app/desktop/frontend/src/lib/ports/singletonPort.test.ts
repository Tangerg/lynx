import { describe, expect, it } from "vitest";
import { createSingletonPort } from "./singletonPort";

describe("createSingletonPort", () => {
  it("fails fast before configuration and after disposal", () => {
    const port = createSingletonPort<{ id: string }>("missing adapter");
    expect(() => port.get()).toThrow("missing adapter");

    const dispose = port.configure({ id: "first" });
    expect(port.get().id).toBe("first");
    dispose();
    expect(() => port.get()).toThrow("missing adapter");
  });

  it("does not let a stale or repeated disposer clear a replacement", () => {
    const port = createSingletonPort<{ id: string }>("missing adapter");
    const disposeFirst = port.configure({ id: "first" });
    const disposeSecond = port.configure({ id: "second" });

    disposeFirst();
    disposeFirst();
    expect(port.get().id).toBe("second");

    disposeSecond();
    expect(() => port.get()).toThrow("missing adapter");
  });
});
