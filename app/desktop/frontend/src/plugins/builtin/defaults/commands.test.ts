// Regression: the default-commands plugin subscribes to the shared `extensions`
// map to mirror workspace views / theme accents into palette commands. Because
// `rebuild` *writes* COMMAND contributions to that same map, a naive
// `state.extensions !== prev.extensions` guard re-fired on its own writes and
// recursed until the stack blew (surfaced as a native `sort` throw deep in the
// substrate read). The fix: only rebuild when the views/accents content
// signature actually changes. These tests lock that in.

import { afterEach, describe, expect, it } from "vitest";
import { definePlugin, loadPlugin, lookupExtensionPoint, usePluginStore } from "@/plugins/sdk";
import { COMMAND, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { defaultCommands } from "./commands";

const NOOP = () => {};

afterEach(() => {
  const { unload, loaded } = usePluginStore.getState();
  for (const name of [...loaded.keys()]) unload(name);
});

const commandIds = () => lookupExtensionPoint(COMMAND).map((c) => c.id);

describe("default-commands view/accent mirror", () => {
  it("does not recurse when a workspace view is contributed after load", async () => {
    await loadPlugin(defaultCommands);

    // Before the fix this contribution kicked off rebuild → register → setState
    // → rebuild → … until the stack overflowed. It must simply resolve.
    await loadPlugin(
      definePlugin({
        name: "test.view-contributor",
        version: "1.0.0",
        setup({ host }) {
          host.extensions.contribute(WORKSPACE_VIEW, {
            id: "demo",
            title: "Demo",
            icon: "spark",
            component: () => null,
          });
        },
      }),
    );

    expect(commandIds()).toContain("view.open.demo");
  });

  it("ignores command-only churn (no rebuild, no recursion)", async () => {
    await loadPlugin(defaultCommands);

    // A plugin that only registers a command must not re-trigger the mirror —
    // the views/accents signature is unchanged, so rebuild is a no-op.
    await loadPlugin(
      definePlugin({
        name: "test.command-contributor",
        version: "1.0.0",
        setup({ host }) {
          host.commands.register({ id: "test.ping", label: "Ping", run: NOOP });
        },
      }),
    );

    const ids = commandIds();
    expect(ids).toContain("test.ping");
    // The static defaults survive (sanity that the subscriber didn't tear them down).
    expect(ids).toContain("composer.focus");
  });
});
