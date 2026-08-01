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

  it("resolves concurrent next() calls in FIFO order", async () => {
    const ch = createPushPullChannel<string>();
    const it = ch.iterator();
    const first = it.next();
    const second = it.next();

    ch.push("first");
    ch.push("second");

    expect(await first).toEqual({ value: "first", done: false });
    expect(await second).toEqual({ value: "second", done: false });
  });

  it("close() resolves waiting next() with done=true", async () => {
    const ch = createPushPullChannel<number>();
    const it = ch.iterator();
    const pending = it.next();
    ch.close();
    expect(await pending).toEqual({ value: undefined, done: true });
  });

  it("close() resolves all waiting next() calls", async () => {
    const ch = createPushPullChannel<number>();
    const it = ch.iterator();
    const first = it.next();
    const second = it.next();

    ch.close();

    expect(await first).toEqual({ value: undefined, done: true });
    expect(await second).toEqual({ value: undefined, done: true });
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

  it("fail() drains buffered values and then rejects iteration", async () => {
    const ch = createPushPullChannel<number>();
    const failure = new Error("upstream stream failed");
    ch.push(10);
    ch.push(20);
    ch.fail(failure);

    const it = ch.iterator();
    expect(await it.next()).toEqual({ value: 10, done: false });
    expect(await it.next()).toEqual({ value: 20, done: false });
    await expect(it.next()).rejects.toBe(failure);
  });

  it("fail() rejects every waiting next() immediately", async () => {
    const ch = createPushPullChannel<number>();
    const it = ch.iterator();
    const first = it.next();
    const second = it.next();
    const failure = new Error("connection lost");

    ch.fail(failure);

    await expect(first).rejects.toBe(failure);
    await expect(second).rejects.toBe(failure);
  });

  it("keeps the first terminal state", async () => {
    const failed = createPushPullChannel<number>();
    const firstFailure = new Error("first failure");
    failed.fail(firstFailure);
    failed.fail(new Error("second failure"));
    failed.close();
    await expect(failed.iterator().next()).rejects.toBe(firstFailure);

    const closed = createPushPullChannel<number>();
    closed.close();
    closed.fail(new Error("too late"));
    expect(await closed.iterator().next()).toEqual({ value: undefined, done: true });
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
