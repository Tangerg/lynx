import type { StreamEvent } from "@/rpc";
import type { Disposable, ToolPreviewComponent } from "./types";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { INITIAL_VIEW_STATE } from "@/plugins/sdk/types/agentView";
import { useConfigStore } from "./config";
import { createHost } from "./host";
import { useNotificationStore } from "./notifications";
import {
  ACCENT,
  AGENT_SOURCE,
  BEFORE_UNLOAD_HANDLER,
  COMMAND,
  COMPOSER_ATTACHMENT_SOURCE,
  COMPOSER_KEY_BINDING,
  COMPOSER_PLACEHOLDER,
  COMPOSER_STATUS,
  DATA_PROVIDER,
  ERROR_FALLBACK,
  LAYOUT_SLOT,
  MESSAGE_ROLE,
  ROUTE,
  SETTINGS_PANE,
  SHORTCUT,
  SLASH_COMMAND,
  THEME,
  TOOL_ACTION,
  TOOL_ICON,
  TOOL_PREVIEW,
  WORK_INDEX_ITEM,
} from "./kernelPoints";
import { normalizeCombo, usePluginStore } from "./registry";
import {
  listRpcAfterHooks,
  listRpcBeforeHooks,
  lookupStreamHandlers,
  lookupCustomHandlers,
  lookupDataProvider,
  lookupExtensionPoint,
  pickAgentSource,
  pickComposerPlaceholder,
  pickPluginErrorFallback,
} from "./selectors";
import {
  lookupExtensionByKey,
  lookupExtensionOwnedEntries,
  lookupExtensionOwner,
} from "./selectors/extensions";

// A throwaway component used for `host.extensions.contribute(TOOL_PREVIEW, …)` calls. It doesn't get
// mounted in these tests — we only care about identity.
const StubPreview: ToolPreviewComponent = () => null;

describe("plugin registry", () => {
  it("addToolPreview stores by fn name + tracks owner", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(TOOL_PREVIEW, StubPreview, { key: "bash" });

    expect(lookupExtensionOwner(TOOL_PREVIEW, "bash")).toBe("alpha");
    expect(lookupExtensionByKey(TOOL_PREVIEW, "bash")).toBe(StubPreview);
  });

  it("registering a second plugin for the same fn logs a warning and overrides", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    try {
      const sinkA: Disposable[] = [];
      const sinkB: Disposable[] = [];
      const AnotherPreview: ToolPreviewComponent = () => null;

      createHost("alpha", sinkA).extensions.contribute(TOOL_PREVIEW, StubPreview, { key: "bash" });
      createHost("beta", sinkB).extensions.contribute(TOOL_PREVIEW, AnotherPreview, {
        key: "bash",
      });

      expect(warn).toHaveBeenCalledOnce();
      expect(warn.mock.calls[0]![0]).toMatch(/beta overrides contribution "bash"/);

      expect(lookupExtensionByKey(TOOL_PREVIEW, "bash")).toBe(AnotherPreview);
      expect(lookupExtensionOwner(TOOL_PREVIEW, "bash")).toBe("beta");
    } finally {
      warn.mockRestore();
    }
  });

  it("disposable.dispose removes the entry — but only if the owner matches", () => {
    const sink: Disposable[] = [];
    const d = createHost("alpha", sink).extensions.contribute(TOOL_PREVIEW, StubPreview, {
      key: "bash",
    });
    expect(lookupExtensionByKey(TOOL_PREVIEW, "bash")).toBeDefined();

    d.dispose();
    expect(lookupExtensionByKey(TOOL_PREVIEW, "bash")).toBeUndefined();
  });

  it("dispose owned by a different plugin is a no-op", () => {
    // First plugin registers; second plugin's stale dispose can't remove it.
    const a: Disposable[] = [];
    const b: Disposable[] = [];
    const hostA = createHost("alpha", a);
    const hostB = createHost("beta", b);

    hostA.extensions.contribute(TOOL_PREVIEW, StubPreview, { key: "bash" });
    // Beta hands back a Disposable that would remove its own entry — but
    // alpha owns the slot, so beta's dispose should not affect alpha.
    const otherDispose = hostB.extensions.contribute(TOOL_PREVIEW, StubPreview, {
      key: "grep",
    }).dispose;
    // overriding the same fn replaces ownership:
    // simulate: after override, then disposing the "old" handle should not
    // touch the new owner's entry.
    hostB.extensions.contribute(TOOL_PREVIEW, StubPreview, { key: "bash" });
    const aDispose = a[a.length - 1]!.dispose; // alpha's "bash" disposer (stale)
    aDispose();

    // beta still owns "bash"
    expect(lookupExtensionOwner(TOOL_PREVIEW, "bash")).toBe("beta");
    // and beta's "grep" handle still works
    otherDispose();
    expect(lookupExtensionByKey(TOOL_PREVIEW, "grep")).toBeUndefined();
  });

  it("settings panes are queryable + ordered via the useSettingsPanes selector", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);

    host.extensions.contribute(SETTINGS_PANE, {
      id: "z",
      label: "Z",
      order: 50,
      component: () => null,
    });
    host.extensions.contribute(SETTINGS_PANE, {
      id: "a",
      label: "A",
      order: 10,
      component: () => null,
    });

    const panes = lookupExtensionPoint(SETTINGS_PANE);

    expect(panes.map((p) => p.id)).toEqual(["a", "z"]);
  });

  it("lookupCustomHandlers returns every registered handler in order", () => {
    const sinkA: Disposable[] = [];
    const sinkB: Disposable[] = [];
    const handlerA = vi.fn();
    const handlerB = vi.fn();
    createHost("alpha", sinkA).events.onCustom("custom.thing", handlerA);
    createHost("beta", sinkB).events.onCustom("custom.thing", handlerB);

    const found = lookupCustomHandlers("custom.thing");
    expect(found.map((h) => h.handler)).toEqual([handlerA, handlerB]);
    expect(found.map((h) => h.pluginName)).toEqual(["alpha", "beta"]);
  });

  it("lookupSlashCommand normalizes the leading slash", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(SLASH_COMMAND, { description: "pong" }, { key: "/ping" });
    host.extensions.contribute(SLASH_COMMAND, { description: "hi" }, { key: "hello" });

    expect(lookupExtensionByKey(SLASH_COMMAND, "/ping")?.description).toBe("pong");
    expect(lookupExtensionByKey(SLASH_COMMAND, "/hello")?.description).toBe("hi");
  });

  it("onCore registers multiple handlers per event type and chains in order", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);

    host.events.onStream("run.started", (s) => ({
      ...s,
      run: { ...s.run, sessionId: "a" },
    }));
    host.events.onStream("run.started", (s) => ({
      ...s,
      run: { ...s.run, sessionId: `${s.run.sessionId}b` },
    }));

    const handlers = lookupStreamHandlers("run.started");
    expect(handlers).toHaveLength(2);

    // Apply by hand to verify ordering.
    let state = INITIAL_VIEW_STATE;
    for (const { handler } of handlers) {
      state = handler(state, {
        type: "run.started",
        run: { id: "r", sessionId: "a" },
      } as StreamEvent);
    }
    expect(state.run.sessionId).toBe("ab");
  });

  it("onCore disposable removes the handler", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);

    const d = host.events.onStream("run.started", (s) => s);
    expect(lookupStreamHandlers("run.started")).toHaveLength(1);

    d.dispose();
    expect(lookupStreamHandlers("run.started")).toHaveLength(0);
  });

  it("layout.register stores the spec under (slot, plugin, id)", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);

    host.layout.register("app.main", { id: "x", order: 5, component: () => null });

    const entries = lookupExtensionOwnedEntries(LAYOUT_SLOT);
    expect(entries).toHaveLength(1);
    expect(entries[0]!.pluginName).toBe("alpha");
    expect(entries[0]!.item.slot).toBe("app.main");
    expect(entries[0]!.item.spec.id).toBe("x");
  });

  it("layout.register disposable removes the slot entry", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const d = host.layout.register("app.main", { id: "x", component: () => null });

    expect(lookupExtensionPoint(LAYOUT_SLOT).length).toBe(1);
    d.dispose();
    expect(lookupExtensionPoint(LAYOUT_SLOT).length).toBe(0);
  });

  it("theme.registerTheme + lookupTheme round-trip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const d = host.extensions.contribute(THEME, { id: "dim", label: "Dim", scheme: "dark" });

    expect(lookupExtensionByKey(THEME, "dim")?.label).toBe("Dim");
    d.dispose();
    expect(lookupExtensionByKey(THEME, "dim")).toBeUndefined();
  });

  it("theme.registerTheme retains the tokens map for applyTheme to consume", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(THEME, {
      id: "dim",
      label: "Dim",
      scheme: "dark",
      tokens: {
        "color-bg": "#101010",
        "color-text": "#fafafa",
      },
    });

    const spec = lookupExtensionByKey(THEME, "dim");
    expect(spec?.tokens).toEqual({
      "color-bg": "#101010",
      "color-text": "#fafafa",
    });
  });

  it("theme.registerAccent + lookupAccent round-trip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(ACCENT, { id: "violet", label: "Violet", dark: "#7c3aed" });

    expect(lookupExtensionByKey(ACCENT, "violet")?.dark).toBe("#7c3aed");
  });

  it("router.register surfaces specs via listRoutes, ordered", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(ROUTE, { id: "b", path: "/b", component: () => null, order: 10 });
    host.extensions.contribute(ROUTE, { id: "a", path: "/a", component: () => null, order: 1 });

    expect(lookupExtensionPoint(ROUTE).map((r) => r.id)).toEqual(["a", "b"]);
  });

  it("normalizeCombo canonicalizes modifier order + case", () => {
    expect(normalizeCombo("Cmd+K")).toBe("mod+k");
    expect(normalizeCombo("cmd+K")).toBe("mod+k");
    expect(normalizeCombo("Shift+Mod+P")).toBe("mod+shift+p");
    expect(normalizeCombo("Escape")).toBe("escape");
    expect(normalizeCombo("ctrl+alt+/")).toBe("ctrl+alt+/");
  });

  it("shortcuts.register stores under the canonical combo", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const handler = vi.fn();

    host.extensions.contribute(SHORTCUT, { key: "Cmd+K", handler });

    // Both canonical and equivalent inputs resolve the same entry.
    expect(lookupExtensionByKey(SHORTCUT, "mod+k")).toBeDefined();
    expect(lookupExtensionByKey(SHORTCUT, normalizeCombo("Mod+K"))?.handler).toBe(handler);
  });

  it("shortcuts.register disposable removes the entry", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const d = host.extensions.contribute(SHORTCUT, { key: "Escape", handler: () => {} });

    expect(lookupExtensionByKey(SHORTCUT, "escape")).toBeDefined();
    d.dispose();
    expect(lookupExtensionByKey(SHORTCUT, "escape")).toBeUndefined();
  });

  it("composer.registerStatus stores a chip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(COMPOSER_STATUS, { id: "branch", order: 5, component: () => null });

    expect(lookupExtensionPoint(COMPOSER_STATUS).length).toBe(1);
    expect(lookupExtensionByKey(COMPOSER_STATUS, "branch")?.order).toBe(5);
  });

  it("workIndex.registerItem stores an expanded item", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(WORK_INDEX_ITEM, {
      id: "bookmarks",
      scope: "session",
      placement: "expanded",
      order: 20,
      component: () => null,
    });

    expect(lookupExtensionPoint(WORK_INDEX_ITEM).length).toBe(1);
    expect(lookupExtensionOwner(WORK_INDEX_ITEM, "bookmarks")).toBe("alpha");
  });

  it("tool.registerIcon + lookupToolIcon round-trip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(TOOL_ICON, "terminal", { key: "bash" });

    expect(lookupExtensionByKey(TOOL_ICON, "bash")).toBe("terminal");
    expect(lookupExtensionByKey(TOOL_ICON, "unknown")).toBeUndefined();
  });

  it("config.set/get round-trip + onChange fires", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const fn = vi.fn();
    host.config.onChange("featureX", fn);

    host.config.set("featureX", true);
    expect(host.config.get("featureX")).toBe(true);
    expect(fn).toHaveBeenCalledOnce();
    expect(fn.mock.calls[0]![0]).toBe(true);

    // Setting the same value doesn't re-fire subscribers.
    host.config.set("featureX", true);
    expect(fn).toHaveBeenCalledOnce();
  });

  it("config.get returns fallback when unset", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    expect(host.config.get("missing", "default")).toBe("default");
    expect(host.config.has("missing")).toBe(false);
  });

  it("config subscriber disposable unwires it", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const fn = vi.fn();
    const d = host.config.onChange("k", fn);
    d.dispose();

    host.config.set("k", 1);
    expect(fn).not.toHaveBeenCalled();

    // Make sure no orphan subscriber set remains.
    const subs = useConfigStore.getState().subscribers.get("k");
    expect(subs).toBeUndefined();
  });

  it("composer.registerKeyBinding stores under canonical key", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Mod+Enter",
      handler: () => true,
    });

    expect(lookupExtensionByKey(COMPOSER_KEY_BINDING, "mod+enter")).toBeDefined();
  });

  it("composer.registerAttachmentSource stores a source", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(COMPOSER_ATTACHMENT_SOURCE, {
      id: "files",
      order: 5,
      useAttachments: () => [],
    });

    expect(lookupExtensionPoint(COMPOSER_ATTACHMENT_SOURCE).length).toBe(1);
    expect(lookupExtensionByKey(COMPOSER_ATTACHMENT_SOURCE, "files")?.order).toBe(5);
  });

  it("tool.registerAction stores an action spec", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(TOOL_ACTION, {
      id: "copy",
      icon: "copy",
      title: "Copy",
      run: () => {},
    });

    expect(lookupExtensionPoint(TOOL_ACTION).length).toBe(1);
    expect(lookupExtensionByKey(TOOL_ACTION, "copy")?.title).toBe("Copy");
  });

  it("state.slice shares store across plugins by name", () => {
    const sink: Disposable[] = [];
    const a = createHost("alpha", sink);
    const b = createHost("beta", sink);

    const sa = a.state.slice<number>("counter", 0);
    const sb = b.state.slice<number>("counter", 999); // initial ignored

    expect(sa.get()).toBe(0);
    expect(sb.get()).toBe(0);
    sa.set(5);
    expect(sb.get()).toBe(5);
  });

  it("window.setTitle/setBadge updates document.title", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);

    host.window.setTitle("My App");
    expect(document.title).toBe("My App");

    host.window.setBadge(3);
    expect(document.title).toBe("(3) My App");

    host.window.setBadge(0);
    expect(document.title).toBe("My App");
  });

  it("plugins.onLoad fires when registerLoaded runs", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const fn = vi.fn();
    host.plugins.onLoad(fn);

    usePluginStore.getState().registerLoaded({
      spec: { name: "beta", version: "1.0.0", setup: () => {} },
      disposables: [],
    });

    expect(fn).toHaveBeenCalledOnce();
    expect(fn.mock.calls[0]![0].name).toBe("beta");
  });

  it("plugins.registerErrorFallback picks highest priority", () => {
    const sink: Disposable[] = [];
    const a = createHost("alpha", sink);
    const b = createHost("beta", sink);

    const lo = () => null;
    const hi = () => null;
    a.extensions.contribute(ERROR_FALLBACK, { id: "lo", priority: 0, component: lo });
    b.extensions.contribute(ERROR_FALLBACK, { id: "hi", priority: 10, component: hi });

    expect(pickPluginErrorFallback()?.id).toBe("hi");
  });

  it("plugins.onUnload fires when unload runs", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const fn = vi.fn();
    host.plugins.onUnload(fn);

    usePluginStore.getState().registerLoaded({
      spec: { name: "beta", version: "1.0.0", setup: () => {} },
      disposables: [],
    });
    usePluginStore.getState().unload("beta");

    expect(fn).toHaveBeenCalledOnce();
    expect(fn.mock.calls[0]![0]).toBe("beta");
  });

  it("state.slice subscribers fire on change + can be disposed", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const slice = host.state.slice<string>("focused", "");
    const listener = vi.fn();
    const d = slice.subscribe(listener);

    slice.set("a.ts");
    slice.set("b.ts");
    expect(listener).toHaveBeenCalledTimes(2);

    d.dispose();
    slice.set("c.ts");
    expect(listener).toHaveBeenCalledTimes(2);
  });

  it("pickAgentSource picks the highest-priority registration", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    // We don't actually need a real AbstractAgent for the picker test — the
    // factory only runs if someone calls it.
    const lo = vi.fn();
    const hi = vi.fn();
    host.extensions.contribute(AGENT_SOURCE, {
      id: "lo",
      label: "Low",
      priority: 0,
      factory: lo as never,
    });
    host.extensions.contribute(AGENT_SOURCE, {
      id: "hi",
      label: "High",
      priority: 10,
      factory: hi as never,
    });

    expect(pickAgentSource()?.id).toBe("hi");
  });

  it("pickAgentSource returns undefined when nothing is registered", () => {
    expect(pickAgentSource()).toBeUndefined();
  });

  it("commands.register stores + lookupCommand retrieves", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const run = vi.fn();
    host.commands.register({ id: "do-it", label: "Do It", run });

    expect(lookupExtensionByKey(COMMAND, "do-it")?.label).toBe("Do It");
    lookupExtensionByKey(COMMAND, "do-it")?.run();
    expect(run).toHaveBeenCalledOnce();
  });

  it("commands.execute runs a registered command by id + forwards args", async () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const run = vi.fn();
    host.commands.register({ id: "echo", label: "Echo", run });

    await host.commands.execute("echo", "a", 1);
    expect(run).toHaveBeenCalledWith("a", 1);
  });

  it("commands.execute on an unknown id warns and no-ops", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const host = createHost("alpha", []);
    await host.commands.execute("nope");
    expect(warn.mock.calls[0]![0]).toMatch(/commands\.execute\("nope"\): no command/);
  });

  it("data.registerProvider + lookupDataProvider round-trip", async () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(DATA_PROVIDER, {
      key: "rows",
      fetcher: async () => [{ id: "a" }, { id: "b" }],
    });

    const fetcher = lookupDataProvider<{ id: string }[]>("rows");
    expect(fetcher).toBeDefined();
    await expect(fetcher!()).resolves.toEqual([{ id: "a" }, { id: "b" }]);
  });

  it("data.registerProvider disposable removes the entry", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const d = host.extensions.contribute(DATA_PROVIDER, { key: "rows", fetcher: async () => [] });

    expect(lookupDataProvider("rows")).toBeDefined();
    d.dispose();
    expect(lookupDataProvider("rows")).toBeUndefined();
  });

  it("workIndex.registerItem stores a rail item", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(WORK_INDEX_ITEM, {
      id: "tools",
      scope: "global",
      placement: "rail",
      order: 900,
      component: () => null,
    });

    expect(lookupExtensionPoint(WORK_INDEX_ITEM).length).toBe(1);
    expect(lookupExtensionByKey(WORK_INDEX_ITEM, "tools")?.order).toBe(900);
  });

  it("message.registerRole stores a role identity", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(MESSAGE_ROLE, { id: "dev", displayName: "Dev", icon: "tool" });

    expect(lookupExtensionPoint(MESSAGE_ROLE).length).toBe(1);
    expect(lookupExtensionByKey(MESSAGE_ROLE, "dev")?.displayName).toBe("Dev");
  });

  it("rpc.beforeRequest + listRpcBeforeHooks round-trip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const hook = vi.fn();
    const d = host.rpc.beforeRequest(hook);

    expect(listRpcBeforeHooks()).toHaveLength(1);
    expect(listRpcBeforeHooks()[0]).toBe(hook);
    d.dispose();
    expect(listRpcBeforeHooks()).toHaveLength(0);
  });

  it("rpc.afterResponse + listRpcAfterHooks round-trip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const hook = vi.fn();
    host.rpc.afterResponse(hook);

    const hooks = listRpcAfterHooks();
    expect(hooks).toHaveLength(1);
    expect(hooks[0]).toBe(hook);
  });

  it("log.info fans events out to every subscriber with plugin attribution", () => {
    const log = vi.spyOn(console, "info").mockImplementation(() => {});
    try {
      const sink: Disposable[] = [];
      const host = createHost("alpha", sink);
      const sub = vi.fn();
      host.log.subscribe(sub);

      host.log.info("hello", 1, 2);

      // Console forwarded with prefix
      expect(log).toHaveBeenCalledOnce();
      expect(log.mock.calls[0]![0]).toBe("[plugin:alpha]");

      // Subscriber received the event
      expect(sub).toHaveBeenCalledOnce();
      const ev = sub.mock.calls[0]![0];
      expect(ev.plugin).toBe("alpha");
      expect(ev.level).toBe("info");
      expect(ev.args).toEqual(["hello", 1, 2]);
      expect(typeof ev.timestamp).toBe("number");
    } finally {
      log.mockRestore();
    }
  });

  it("lifecycle.onReady queues + fires on markAppReady", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const fn = vi.fn();
    host.lifecycle.onReady(fn);

    expect(fn).not.toHaveBeenCalled();
    usePluginStore.getState().markAppReady();
    expect(fn).toHaveBeenCalledOnce();
  });

  it("lifecycle.onReady fires on next microtask when already ready", async () => {
    usePluginStore.getState().markAppReady();
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const fn = vi.fn();
    host.lifecycle.onReady(fn);

    // Not synchronous — queued.
    expect(fn).not.toHaveBeenCalled();
    await Promise.resolve();
    expect(fn).toHaveBeenCalledOnce();
  });

  it("lifecycle.onBeforeUnload registration is removable", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const fn = vi.fn();
    const d = host.lifecycle.onBeforeUnload(fn);

    expect(lookupExtensionPoint(BEFORE_UNLOAD_HANDLER).length).toBe(1);
    d.dispose();
    expect(lookupExtensionPoint(BEFORE_UNLOAD_HANDLER).length).toBe(0);
  });

  it("notify() pushes to the persistent feed", () => {
    const info = vi.spyOn(console, "info").mockImplementation(() => {});
    try {
      const sink: Disposable[] = [];
      const host = createHost("alpha", sink);
      host.notify("hello world");

      const log = useNotificationStore.getState().log;
      expect(log).toHaveLength(1);
      expect(log[0]).toMatchObject({
        plugin: "alpha",
        level: "info",
        message: "hello world",
      });
    } finally {
      info.mockRestore();
    }
  });

  it("composer.registerPlaceholder pool picks one entry", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(COMPOSER_PLACEHOLDER, { id: "only", text: "Hi" });

    const pick = pickComposerPlaceholder();
    expect(pick?.text).toBe("Hi");
  });

  it("pickComposerPlaceholder returns undefined when nothing registered", () => {
    expect(pickComposerPlaceholder()).toBeUndefined();
  });

  it("log subscriber that throws does not break the logger", () => {
    const err = vi.spyOn(console, "error").mockImplementation(() => {});
    const info = vi.spyOn(console, "info").mockImplementation(() => {});
    try {
      const sink: Disposable[] = [];
      const host = createHost("alpha", sink);
      host.log.subscribe(() => {
        throw new Error("nope");
      });
      const good = vi.fn();
      host.log.subscribe(good);

      // Should not throw.
      host.log.info("hi");

      // Second subscriber still ran.
      expect(good).toHaveBeenCalledOnce();
      // The throw was surfaced via console.error.
      expect(err).toHaveBeenCalled();
    } finally {
      info.mockRestore();
      err.mockRestore();
    }
  });
});

describe("registry.unload", () => {
  it("disposes every handle a plugin registered", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(TOOL_PREVIEW, StubPreview, { key: "bash" });
    host.extensions.contribute(TOOL_PREVIEW, StubPreview, { key: "grep" });

    usePluginStore.getState().registerLoaded({
      spec: { name: "alpha", version: "1.0.0", setup: () => {} },
      disposables: sink,
    });

    usePluginStore.getState().unload("alpha");

    expect(usePluginStore.getState().loaded.has("alpha")).toBe(false);
    expect(lookupExtensionPoint(TOOL_PREVIEW).length).toBe(0);
  });

  it("dispose throwing in one handle doesn't abort the rest", () => {
    const err = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      const sink: Disposable[] = [
        {
          dispose: () => {
            throw new Error("nope");
          },
        },
      ];
      const host = createHost("alpha", sink);
      host.extensions.contribute(TOOL_PREVIEW, StubPreview, { key: "bash" });

      usePluginStore.getState().registerLoaded({
        spec: { name: "alpha", version: "1.0.0", setup: () => {} },
        disposables: sink,
      });

      usePluginStore.getState().unload("alpha");

      // The good disposer still ran — "bash" is gone.
      expect(lookupExtensionPoint(TOOL_PREVIEW).length).toBe(0);
      // The bad one was logged.
      expect(err).toHaveBeenCalled();
    } finally {
      err.mockRestore();
    }
  });
});

// Reset spies between specs.
beforeEach(() => vi.restoreAllMocks());
afterEach(() => vi.restoreAllMocks());
