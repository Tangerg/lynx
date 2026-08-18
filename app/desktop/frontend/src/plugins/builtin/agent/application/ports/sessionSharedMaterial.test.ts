import { afterEach, describe, expect, it } from "vitest";
import {
  registerAgentSessionSharedMaterial,
  stageAgentSessionSharedMaterial,
} from "./sessionSharedMaterial";

const disposals: Array<() => void> = [];

afterEach(() => {
  for (const dispose of disposals.splice(0).reverse()) dispose();
});

describe("Session shared material contributors", () => {
  it("projects a bounded context into the winning Agent material value", () => {
    disposals.push(
      registerAgentSessionSharedMaterial<{ revision: number }>("companion", (sessionId, value) => ({
        sessionId,
        revision: value.revision,
      })),
    );
    const project = stageAgentSessionSharedMaterial("ses_1", { revision: 4 });

    expect(project({ plan: { revision: 2 } })).toEqual({
      plan: { revision: 2 },
      companion: { sessionId: "ses_1", revision: 4 },
    });
  });

  it("withdraws a staged value when its plugin owner retires before commit", () => {
    const dispose = registerAgentSessionSharedMaterial<{ revision: number }>(
      "companion",
      (_sessionId, value) => value,
    );
    const project = stageAgentSessionSharedMaterial("ses_1", { revision: 4 });

    dispose();

    expect(project({ plan: { revision: 2 } })).toEqual({ plan: { revision: 2 } });
  });

  it("rejects a second owner for the same material key", () => {
    disposals.push(registerAgentSessionSharedMaterial("companion", () => null));

    expect(() => registerAgentSessionSharedMaterial("companion", () => null)).toThrow(
      'Agent Session shared material "companion" already has an owner',
    );
  });
});
