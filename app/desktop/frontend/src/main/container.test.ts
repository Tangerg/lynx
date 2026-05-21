import { describe, it, expect, afterEach } from "vitest";
import type { ApprovalSubmission, PermissionGateway } from "@/domain";
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
});
