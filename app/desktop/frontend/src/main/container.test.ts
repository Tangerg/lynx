import type { ApprovalSubmission, PermissionGateway } from "@/domain";
import { afterEach, describe, expect, it } from "vitest";
import { getContainer, resetContainer, setContainer } from "./container";

class FakePermissionGateway implements PermissionGateway {
  calls: ApprovalSubmission[] = [];
  async submit(s: ApprovalSubmission) {
    this.calls.push(s);
  }
}

describe("main/container", () => {
  afterEach(resetContainer);

  it("exposes a default permission gateway out of the box", () => {
    expect(getContainer().permission).toBeDefined();
    expect(typeof getContainer().permission.submit).toBe("function");
  });

  it("setContainer() swaps a single gateway, leaving others intact", () => {
    const fake = new FakePermissionGateway();
    setContainer({ permission: fake });
    expect(getContainer().permission).toBe(fake);
  });

  it("resetContainer() restores defaults", () => {
    const fake = new FakePermissionGateway();
    setContainer({ permission: fake });
    resetContainer();
    expect(getContainer().permission).not.toBe(fake);
  });

  it("gateway calls route through whatever container currently holds", async () => {
    const fake = new FakePermissionGateway();
    setContainer({ permission: fake });
    await getContainer().permission.submit({ requestId: "r1", decision: "approved" });
    expect(fake.calls).toEqual([{ requestId: "r1", decision: "approved" }]);
  });

  it("methods() returns a cached singleton (one client for the container's life)", () => {
    const first = getContainer().methods();
    expect(getContainer().methods()).toBe(first);
    // A fresh container builds a new one.
    resetContainer();
    expect(getContainer().methods()).not.toBe(first);
  });
});
