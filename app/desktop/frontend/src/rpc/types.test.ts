import { describe, expect, it } from "vitest";
import {
  isErrorResponse,
  isNotification,
  isRequest,
  isResponse,
  JSONRPC_VERSION,
  RPC_METHOD_NOT_FOUND,
  RPC_SESSION_NOT_FOUND,
} from "./types";

describe("rpc/types discriminators", () => {
  it("isRequest matches { jsonrpc, id, method, params? }", () => {
    expect(isRequest({ jsonrpc: JSONRPC_VERSION, id: 1, method: "x" })).toBe(true);
    expect(isRequest({ jsonrpc: JSONRPC_VERSION, id: "a", method: "x", params: {} })).toBe(true);
  });

  it("isResponse matches { jsonrpc, id, result|error } but not Request", () => {
    expect(isResponse({ jsonrpc: JSONRPC_VERSION, id: 1, result: null })).toBe(true);
    expect(
      isResponse({
        jsonrpc: JSONRPC_VERSION,
        id: 2,
        error: { code: RPC_METHOD_NOT_FOUND, message: "no" },
      }),
    ).toBe(true);
    // A request has both id AND method — must not be classified as Response.
    expect(isResponse({ jsonrpc: JSONRPC_VERSION, id: 1, method: "x" })).toBe(false);
  });

  it("isNotification matches { jsonrpc, method } with no id", () => {
    expect(isNotification({ jsonrpc: JSONRPC_VERSION, method: "notifications/run/event" })).toBe(
      true,
    );
    expect(
      isNotification({
        jsonrpc: JSONRPC_VERSION,
        method: "notifications/run/event",
        params: { x: 1 },
      }),
    ).toBe(true);
    // A Response has id — not a Notification even if method missing.
    expect(isNotification({ jsonrpc: JSONRPC_VERSION, id: 7, result: 1 })).toBe(false);
  });

  it("isErrorResponse splits success vs failure Response", () => {
    expect(isErrorResponse({ jsonrpc: JSONRPC_VERSION, id: 1, result: 1 })).toBe(false);
    expect(
      isErrorResponse({
        jsonrpc: JSONRPC_VERSION,
        id: 1,
        error: { code: RPC_SESSION_NOT_FOUND, message: "no" },
      }),
    ).toBe(true);
  });
});
