import { describe, expect, it } from "vitest";
import {
  errorDetail,
  errorRetryAfterSeconds,
  errorType,
  isErrorResponse,
  isNotification,
  isResponse,
  JSONRPC_VERSION,
  parseRpcMessage,
  RPC_METHOD_NOT_FOUND,
} from "./types";

describe("rpc/types discriminators", () => {
  it("isResponse matches { jsonrpc, id, result|error } but not Request", () => {
    expect(isResponse({ jsonrpc: JSONRPC_VERSION, id: "1", result: null })).toBe(true);
    expect(
      isResponse({
        jsonrpc: JSONRPC_VERSION,
        id: "2",
        error: { code: RPC_METHOD_NOT_FOUND, message: "no" },
      }),
    ).toBe(true);
    // A request has both id AND method — must not be classified as Response.
    expect(isResponse({ jsonrpc: JSONRPC_VERSION, id: "1", method: "x" })).toBe(false);
  });

  it("isNotification matches { jsonrpc, method } with no id", () => {
    expect(
      isNotification({ jsonrpc: JSONRPC_VERSION, method: "notifications.example.event" }),
    ).toBe(true);
    expect(
      isNotification({
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.example.event",
        params: { x: 1 },
      }),
    ).toBe(true);
    // A Response has id — not a Notification even if method missing.
    expect(isNotification({ jsonrpc: JSONRPC_VERSION, id: "7", result: 1 })).toBe(false);
  });

  it("isErrorResponse splits success vs failure Response", () => {
    expect(isErrorResponse({ jsonrpc: JSONRPC_VERSION, id: "1", result: 1 })).toBe(false);
    expect(
      isErrorResponse({
        jsonrpc: JSONRPC_VERSION,
        id: "1",
        // A business code, written as the number it is: this client names only the
        // five standard ones, and the envelope gate cares about the shape.
        error: { code: -32002, message: "no" },
      }),
    ).toBe(true);
  });
});

describe("parseRpcMessage envelope gate", () => {
  it("accepts each well-formed envelope kind and passes the payload through opaque", () => {
    const resp = parseRpcMessage(`{"jsonrpc":"2.0","id":"1","result":{"ok":true,"n":3}}`);
    expect(resp).toEqual({ jsonrpc: "2.0", id: "1", result: { ok: true, n: 3 } });
    expect(parseRpcMessage(`{"jsonrpc":"2.0","id":"2","method":"x","params":{"a":1}}`)).toEqual({
      jsonrpc: "2.0",
      id: "2",
      method: "x",
      params: { a: 1 },
    });
    expect(
      parseRpcMessage(
        `{"jsonrpc":"2.0","method":"notifications.run.event","params":{"runId":"r"}}`,
      ),
    ).toMatchObject({ method: "notifications.run.event" });
    expect(
      parseRpcMessage(`{"jsonrpc":"2.0","id":"3","error":{"code":-32002,"message":"gone"}}`),
    ).toMatchObject({ error: { code: -32002, message: "gone" } });
  });

  it("rejects invalid JSON", () => {
    expect(parseRpcMessage("not json")).toBeNull();
    expect(parseRpcMessage("{unterminated")).toBeNull();
  });

  it("rejects non-envelopes (wrong/missing jsonrpc, non-objects)", () => {
    expect(parseRpcMessage(`{"id":"1","result":1}`)).toBeNull(); // no jsonrpc
    expect(parseRpcMessage(`{"jsonrpc":"1.0","id":"1","result":1}`)).toBeNull(); // wrong version
    expect(parseRpcMessage(`{"jsonrpc":"2.0","error":{"message":"no code"}}`)).toBeNull(); // malformed error
    expect(parseRpcMessage(`[1,2,3]`)).toBeNull();
    expect(parseRpcMessage(`"a string"`)).toBeNull();
    expect(parseRpcMessage(`42`)).toBeNull();
    expect(parseRpcMessage(`null`)).toBeNull();
  });

  it("rejects ambiguous or incomplete envelope shapes", () => {
    expect(parseRpcMessage(`{"jsonrpc":"2.0","id":"1"}`)).toBeNull();
    expect(
      parseRpcMessage(
        `{"jsonrpc":"2.0","id":"1","result":{},"error":{"code":-32603,"message":"no"}}`,
      ),
    ).toBeNull();
    expect(parseRpcMessage(`{"jsonrpc":"2.0","id":"1","method":"x","result":{}}`)).toBeNull();
    expect(parseRpcMessage(`{"jsonrpc":"2.0","id":"1","result":{},"params":{}}`)).toBeNull();
    expect(
      parseRpcMessage(`{"jsonrpc":"2.0","method":"x","error":{"code":1,"message":"no"}}`),
    ).toBeNull();
    expect(
      parseRpcMessage(`{"jsonrpc":"2.0","id":"1","error":{"code":1.5,"message":"no"}}`),
    ).toBeNull();
  });
});

// errorDetail reports whether the runtime said anything about this occurrence.
// It once fell back to the symbolic type, so it could never answer "nothing" —
// which handed every caller a bare symbol to show and pre-empted the layers that
// own the words.
describe("errorDetail reports only what the runtime said", () => {
  it("returns the per-occurrence detail", () => {
    expect(errorDetail({ type: "tool_failed", detail: "exit status 2" })).toBe("exit status 2");
  });

  it("returns undefined when there is no detail, symbol or not", () => {
    expect(errorDetail({ type: "session_busy" })).toBeUndefined();
    expect(errorDetail({ type: "session_busy", detail: "" })).toBeUndefined();
    expect(errorDetail({})).toBeUndefined();
    expect(errorDetail(undefined)).toBeUndefined();
  });

  it("leaves the symbol to errorType, which is what branch logic reads", () => {
    expect(errorType({ type: "session_busy" })).toBe("session_busy");
  });
});

describe("errorRetryAfterSeconds", () => {
  it("accepts only a positive integer delay", () => {
    expect(errorRetryAfterSeconds({ retryAfterSeconds: 3 })).toBe(3);
    expect(errorRetryAfterSeconds({ retryAfterSeconds: 0 })).toBeUndefined();
    expect(errorRetryAfterSeconds({ retryAfterSeconds: -1 })).toBeUndefined();
    expect(errorRetryAfterSeconds({ retryAfterSeconds: 1.5 })).toBeUndefined();
    expect(errorRetryAfterSeconds({ retryAfterSeconds: "3" })).toBeUndefined();
    expect(errorRetryAfterSeconds(undefined)).toBeUndefined();
  });
});
