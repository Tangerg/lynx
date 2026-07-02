import { describe, expect, it } from "vitest";
import {
  isErrorResponse,
  isNotification,
  isRequest,
  isResponse,
  JSONRPC_VERSION,
  parseRpcMessage,
  RPC_METHOD_NOT_FOUND,
  RPC_SESSION_NOT_FOUND,
} from "./types";

describe("rpc/types discriminators", () => {
  it("isRequest matches { jsonrpc, id, method, params? }", () => {
    expect(isRequest({ jsonrpc: JSONRPC_VERSION, id: "1", method: "x" })).toBe(true);
    expect(isRequest({ jsonrpc: JSONRPC_VERSION, id: "42", method: "x", params: {} })).toBe(true);
  });

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
    expect(isNotification({ jsonrpc: JSONRPC_VERSION, id: "7", result: 1 })).toBe(false);
  });

  it("isErrorResponse splits success vs failure Response", () => {
    expect(isErrorResponse({ jsonrpc: JSONRPC_VERSION, id: "1", result: 1 })).toBe(false);
    expect(
      isErrorResponse({
        jsonrpc: JSONRPC_VERSION,
        id: "1",
        error: { code: RPC_SESSION_NOT_FOUND, message: "no" },
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
});
