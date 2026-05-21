import { describe, it, expect, afterEach } from "vitest";
import type { ApprovalSubmission, PermissionGateway } from "@/domain";
import { getContainer, setContainer } from "./container";

class FakePermissionGateway implements PermissionGateway {
  calls: ApprovalSubmission[] = [];
  async submit(s: ApprovalSubmission) {
    this.calls.push(s);
  }
}

describe("main/container", () => {
  afterEach(() => setContainer(null));

  it("exposes a default permission gateway out of the box", () => {
    expect(getContainer().permission).toBeDefined();
    expect(typeof getContainer().permission.submit).toBe("function");
  });

  it("setContainer() swaps a single gateway, leaving others intact", () => {
    const fake = new FakePermissionGateway();
    setContainer({ permission: fake });
    expect(getContainer().permission).toBe(fake);
  });

  it("setContainer(null) resets to defaults", () => {
    const fake = new FakePermissionGateway();
    setContainer({ permission: fake });
    setContainer(null);
    expect(getContainer().permission).not.toBe(fake);
  });

  it("the swapped gateway is what useApprovalSubmit will call", async () => {
    const fake = new FakePermissionGateway();
    setContainer({ permission: fake });
    await getContainer().permission.submit({ requestId: "r1", decision: "approved" });
    expect(fake.calls).toEqual([{ requestId: "r1", decision: "approved" }]);
  });
});
