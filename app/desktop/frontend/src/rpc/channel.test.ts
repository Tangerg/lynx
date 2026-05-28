import { describe, expect, it } from "vitest";
import { createPushPullChannel } from "./channel";

describe("createPushPullChannel", () => {
  it("yields pushed values in FIFO order", async () => {
    const ch = createPushPullChannel<number>();
    ch.push(1);
    ch.push(2);
    ch.push(3);
    const it = ch.iterator();
    expect(await it.next()).toEqual({ value: 1, done: false });
    expect(await it.next()).toEqual({ value: 2, done: false });
    expect(await it.next()).toEqual({ value: 3, done: false });
  });

  it("blocks next() until a push arrives", async () => {
    const ch = createPushPullChannel<string>();
    const it = ch.iterator();
    const pending = it.next();
    let resolved = false;
    void pending.then(() => {
      resolved = true;
    });
    await Promise.resolve();
    expect(resolved).toBe(false);
    ch.push("delayed");
    expect(await pending).toEqual({ value: "delayed", done: false });
  });

  it("close() resolves waiting next() with done=true", async () => {
    const ch = createPushPullChannel<number>();
    const it = ch.iterator();
    const pending = it.next();
    ch.close();
    expect(await pending).toEqual({ value: undefined, done: true });
  });

  it("buffered values drain before close-driven done", async () => {
    const ch = createPushPullChannel<number>();
    ch.push(10);
    ch.push(20);
    ch.close();
    const it = ch.iterator();
    expect(await it.next()).toEqual({ value: 10, done: false });
    expect(await it.next()).toEqual({ value: 20, done: false });
    expect(await it.next()).toEqual({ value: undefined, done: true });
  });

  it("push after close is silently dropped", async () => {
    const ch = createPushPullChannel<number>();
    ch.close();
    ch.push(99);
    const it = ch.iterator();
    expect(await it.next()).toEqual({ value: undefined, done: true });
  });

  it("close() is idempotent", () => {
    const ch = createPushPullChannel<number>();
    ch.close();
    ch.close();
    expect(ch.closed).toBe(true);
  });

  it("iterator.return() closes the channel", async () => {
    const ch = createPushPullChannel<number>();
    const it = ch.iterator();
    expect(ch.closed).toBe(false);
    await it.return!();
    expect(ch.closed).toBe(true);
  });

  it("for-await drains then exits on close", async () => {
    const ch = createPushPullChannel<string>();
    ch.push("a");
    ch.push("b");
    setTimeout(() => {
      ch.push("c");
      ch.close();
    }, 0);
    const collected: string[] = [];
    for await (const v of ch.iterator()) collected.push(v);
    expect(collected).toEqual(["a", "b", "c"]);
  });
});
