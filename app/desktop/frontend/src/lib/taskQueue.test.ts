import { describe, expect, it } from "vitest";
import { RetirableTaskCohort } from "./taskQueue";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

describe("retirable task cohort", () => {
  it("retires only pending cohort settlements and ignores non-cooperative late results", async () => {
    const retired = new Error("generation retired");
    const cohort = new RetirableTaskCohort(retired);
    await expect(cohort.settle(Promise.resolve("completed"))).resolves.toBe("completed");

    const late = deferred<string>();
    const settlement = cohort.settle(late.promise);
    cohort.retire();
    await expect(settlement).rejects.toBe(retired);

    late.resolve("stale");
    await Promise.resolve();
    expect(() => cohort.assertCurrent()).toThrow(retired);
  });
});
