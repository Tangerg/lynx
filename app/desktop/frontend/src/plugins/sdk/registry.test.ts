import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { EventType, type BaseEvent } from "@ag-ui/core";
import { useConfigStore } from "./config";
import { createHost } from "./host";
import { useNotificationStore } from "./notifications";
import { normalizeCombo, usePluginStore } from "./registry";
import {
  listRoutes,
  listRpcAfterHooks,
  listRpcBeforeHooks,
  lookupAccent,
  lookupCommand,
  lookupComposerKeyBinding,
  lookupCoreEventHandlers,
  lookupCustomEventHandler,
  lookupDataProvider,
  lookupShortcut,
  lookupSlashCommand,
  lookupTheme,
  lookupToolIcon,
  pickAgentSource,
  pickComposerPlaceholder,
  pickPluginErrorFallback,
} from "./selectors";
import { INITIAL_VIEW_STATE } from "@/protocol/agui/viewState";
import type { Disposable, ToolPreviewComponent } from "./types";

// A throwaway component used for `tool.registerPreview` calls. It doesn't get
// mounted in these tests — we only care about identity.
const StubPreview: ToolPreviewComponent = () => null;

describe("plugin registry", () => {
  it("addToolPreview stores by fn name + tracks owner", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.tool.registerPreview("bash", StubPreview);

    const entry = usePluginStore.getState().toolPreviews.get("bash");
    expect(entry?.pluginName).toBe("alpha");
    expect(entry?.value).toBe(StubPreview);
  });

  it("registering a second plugin for the same fn logs a warning and overrides", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    try {
      const sinkA: Disposable[] = [];
      const sinkB: Disposable[] = [];
      const AnotherPreview: ToolPreviewComponent = () => null;

      createHost("alpha", sinkA).tool.registerPreview("bash", StubPreview);
      createHost("beta", sinkB).tool.registerPreview("bash", AnotherPreview);

      expect(warn).toHaveBeenCalledOnce();
      expect(warn.mock.calls[0][0]).toMatch(/beta overrides tool preview "bash"/);

      const entry = usePluginStore.getState().toolPreviews.get("bash");
      expect(entry?.value).toBe(AnotherPreview);
      expect(entry?.pluginName).toBe("beta");
    } finally {
      warn.mockRestore();
    }
  });

  it("Disposable.dispose removes the entry — but only if the owner matches", () => {
    const sink: Disposable[] = [];
    const d = createHost("alpha", sink).tool.registerPreview("bash", StubPreview);
    expect(usePluginStore.getState().toolPreviews.has("bash")).toBe(true);

    d.dispose();
    expect(usePluginStore.getState().toolPreviews.has("bash")).toBe(false);
  });

  it("dispose owned by a different plugin is a no-op", () => {
    // First plugin registers; second plugin's stale dispose can't remove it.
    const a: Disposable[] = [];
    const b: Disposable[] = [];
    const hostA = createHost("alpha", a);
    const hostB = createHost("beta", b);

    hostA.tool.registerPreview("bash", StubPreview);
    // Beta hands back a Disposable that would remove its own entry — but
    // alpha owns the slot, so beta's dispose should not affect alpha.
    const otherDispose = hostB.tool.registerPreview("grep", StubPreview).dispose;
    // overriding the same fn replaces ownership:
    // simulate: after override, then disposing the "old" handle should not
    // touch the new owner's entry.
    hostB.tool.registerPreview("bash", StubPreview);
    const aDispose = a[a.length - 1].dispose; // alpha's "bash" disposer (stale)
    aDispose();

    // beta still owns "bash"
    expect(usePluginStore.getState().toolPreviews.get("bash")?.pluginName).toBe("beta");
    // and beta's "grep" handle still works
    otherDispose();
    expect(usePluginStore.getState().toolPreviews.has("grep")).toBe(false);
  });

  it("settings panes are queryable + ordered via the useSettingsPanes selector", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);

    host.settings.registerPane({ id: "z", label: "Z", order: 50, component: () => null });
    host.settings.registerPane({ id: "a", label: "A", order: 10, component: () => null });

    const panes = Array.from(usePluginStore.getState().settingsPanes.values())
      .map((o) => o.value)
      .sort((x, y) => (x.order ?? 100) - (y.order ?? 100));

    expect(panes.map((p) => p.id)).toEqual(["a", "z"]);
  });

  it("lookupCustomEventHandler returns registered handler", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const handler = vi.fn();
    host.agui.on("custom.thing", handler);

    expect(lookupCustomEventHandler("custom.thing")).toBe(handler);
  });

  it("lookupSlashCommand normalizes the leading slash", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.composer.registerCommand("/ping", { description: "pong" });
    host.composer.registerCommand("hello", { description: "hi" });

    expect(lookupSlashCommand("/ping")?.description).toBe("pong");
    expect(lookupSlashCommand("/hello")?.description).toBe("hi");
  });

  it("onCore registers multiple handlers per event type and chains in order", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);

    host.agui.onCore(EventType.RUN_STARTED, (s) => ({
      ...s,
      run: { ...s.run, threadId: "a" },
    }));
    host.agui.onCore(EventType.RUN_STARTED, (s) => ({
      ...s,
      run: { ...s.run, threadId: s.run.threadId + "b" },
    }));

    const handlers = lookupCoreEventHandlers(EventType.RUN_STARTED);
    expect(handlers).toHaveLength(2);

    // Apply by hand to verify ordering.
    let state = INITIAL_VIEW_STATE;
    for (const { handler } of handlers) {
      state = handler(state, { type: EventType.RUN_STARTED } as BaseEvent);
    }
    expect(state.run.threadId).toBe("ab");
  });

  it("onCore disposable removes the handler", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);

    const d = host.agui.onCore(EventType.RUN_STARTED, (s) => s);
    expect(lookupCoreEventHandlers(EventType.RUN_STARTED)).toHaveLength(1);

    d.dispose();
    expect(lookupCoreEventHandlers(EventType.RUN_STARTED)).toHaveLength(0);
  });

  it("layout.register stores the spec under (slot, plugin, id)", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);

    host.layout.register("app.main", { id: "x", order: 5, component: () => null });

    const entries = Array.from(usePluginStore.getState().layoutSlots.values());
    expect(entries).toHaveLength(1);
    expect(entries[0].pluginName).toBe("alpha");
    expect(entries[0].value.slot).toBe("app.main");
    expect(entries[0].value.spec.id).toBe("x");
  });

  it("layout.register disposable removes the slot entry", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const d = host.layout.register("app.main", { id: "x", component: () => null });

    expect(usePluginStore.getState().layoutSlots.size).toBe(1);
    d.dispose();
    expect(usePluginStore.getState().layoutSlots.size).toBe(0);
  });

  it("theme.registerTheme + lookupTheme round-trip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const d = host.theme.registerTheme({ id: "dim", label: "Dim", scheme: "dark" });

    expect(lookupTheme("dim")?.label).toBe("Dim");
    d.dispose();
    expect(lookupTheme("dim")).toBeUndefined();
  });

  it("theme.registerTheme retains the tokens map for applyTheme to consume", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.theme.registerTheme({
      id: "dim",
      label: "Dim",
      scheme: "dark",
      tokens: {
        "color-bg": "#101010",
        "color-text": "#fafafa",
      },
    });

    const spec = lookupTheme("dim");
    expect(spec?.tokens).toEqual({
      "color-bg": "#101010",
      "color-text": "#fafafa",
    });
  });

  it("theme.registerAccent + lookupAccent round-trip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.theme.registerAccent({ id: "violet", label: "Violet", dark: "#7c3aed" });

    expect(lookupAccent("violet")?.dark).toBe("#7c3aed");
  });

  it("router.register surfaces specs via listRoutes, ordered", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.router.register({ id: "b", path: "/b", component: () => null, order: 10 });
    host.router.register({ id: "a", path: "/a", component: () => null, order: 1 });

    expect(listRoutes().map((r) => r.id)).toEqual(["a", "b"]);
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

    host.shortcuts.register({ key: "Cmd+K", handler });

    // Both canonical and equivalent inputs resolve the same entry.
    expect(lookupShortcut("mod+k")).toBeDefined();
    expect(lookupShortcut(normalizeCombo("Mod+K"))?.handler).toBe(handler);
  });

  it("shortcuts.register disposable removes the entry", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const d = host.shortcuts.register({ key: "Escape", handler: () => {} });

    expect(lookupShortcut("escape")).toBeDefined();
    d.dispose();
    expect(lookupShortcut("escape")).toBeUndefined();
  });

  it("composer.registerStatus stores a chip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.composer.registerStatus({ id: "branch", order: 5, component: () => null });

    const map = usePluginStore.getState().composerStatus;
    expect(map.size).toBe(1);
    expect(map.get("branch")?.value.order).toBe(5);
  });

  it("sidebar.registerSection stores a section", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.sidebar.registerSection({ id: "bookmarks", order: 20, component: () => null });

    const map = usePluginStore.getState().sidebarSections;
    expect(map.size).toBe(1);
    expect(map.get("bookmarks")?.pluginName).toBe("alpha");
  });

  it("tool.registerIcon + lookupToolIcon round-trip", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.tool.registerIcon("bash", "terminal");

    expect(lookupToolIcon("bash")).toBe("terminal");
    expect(lookupToolIcon("unknown")).toBeUndefined();
  });

  it("config.set/get round-trip + onChange fires", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    const fn = vi.fn();
    host.config.onChange("featureX", fn);

    host.config.set("featureX", true);
    expect(host.config.get("featureX")).toBe(true);
    expect(fn).toHaveBeenCalledOnce();
    expect(fn.mock.calls[0][0]).toBe(true);

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
    host.composer.registerKeyBinding({
      key: "Mod+Enter",
      handler: () => true,
    });

    expect(lookupComposerKeyBinding("mod+enter")).toBeDefined();
  });

  it("composer.registerAttachmentSource stores a source", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.composer.registerAttachmentSource({
      id: "files",
      order: 5,
      useAttachments: () => [],
    });

    const map = usePluginStore.getState().composerAttachmentSources;
    expect(map.size).toBe(1);
    expect(map.get("files")?.value.order).toBe(5);
  });

  it("tool.registerAction stores an action spec", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.tool.registerAction({
      id: "copy",
      icon: "copy",
      title: "Copy",
      run: () => {},
    });

    const map = usePluginStore.getState().toolActions;
    expect(map.size).toBe(1);
    expect(map.get("copy")?.value.title).toBe("Copy");
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
    expect(fn.mock.calls[0][0].name).toBe("beta");
  });

  it("plugins.registerErrorFallback picks highest priority", () => {
    const sink: Disposable[] = [];
    const a = createHost("alpha", sink);
    const b = createHost("beta", sink);

    const lo = () => null;
    const hi = () => null;
    a.plugins.registerErrorFallback({ id: "lo", priority: 0, component: lo });
    b.plugins.registerErrorFallback({ id: "hi", priority: 10, component: hi });

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
    expect(fn.mock.calls[0][0]).toBe("beta");
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

  it("composer.registerMode stores a mode", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.composer.registerMode({ id: "research", label: "Research", icon: "search", order: 5 });

    const map = usePluginStore.getState().composerModes;
    expect(map.size).toBe(1);
    expect(map.get("research")?.value.label).toBe("Research");
  });

  it("pickAgentSource picks the highest-priority registration", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    // We don't actually need a real AbstractAgent for the picker test — the
    // factory only runs if someone calls it.
    const lo = vi.fn();
    const hi = vi.fn();
    host.agent.registerSource({ id: "lo", label: "Low", priority: 0, factory: lo as never });
    host.agent.registerSource({ id: "hi", label: "High", priority: 10, factory: hi as never });

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

    expect(lookupCommand("do-it")?.label).toBe("Do It");
    lookupCommand("do-it")?.run();
    expect(run).toHaveBeenCalledOnce();
  });

  it("data.registerProvider + lookupDataProvider round-trip", async () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.data.registerProvider<{ id: string }[]>({
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
    const d = host.data.registerProvider({ key: "rows", fetcher: async () => [] });

    expect(lookupDataProvider("rows")).toBeDefined();
    d.dispose();
    expect(lookupDataProvider("rows")).toBeUndefined();
  });

  it("sidebar.registerRailItem stores an item", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.sidebar.registerRailItem({ id: "tools", order: 900, component: () => null });

    const map = usePluginStore.getState().sidebarRailItems;
    expect(map.size).toBe(1);
    expect(map.get("tools")?.value.order).toBe(900);
  });

  it("message.registerRole stores a role identity", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.message.registerRole({ id: "dev", displayName: "Dev", icon: "tool" });

    const map = usePluginStore.getState().messageRoles;
    expect(map.size).toBe(1);
    expect(map.get("dev")?.value.displayName).toBe("Dev");
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
      expect(log.mock.calls[0][0]).toBe("[plugin:alpha]");

      // Subscriber received the event
      expect(sub).toHaveBeenCalledOnce();
      const ev = sub.mock.calls[0][0];
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

    expect(usePluginStore.getState().beforeUnloadHandlers.size).toBe(1);
    d.dispose();
    expect(usePluginStore.getState().beforeUnloadHandlers.size).toBe(0);
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
    host.composer.registerPlaceholder({ id: "only", text: "Hi" });

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
    host.tool.registerPreview("bash", StubPreview);
    host.tool.registerPreview("grep", StubPreview);

    usePluginStore.getState().registerLoaded({
      spec: { name: "alpha", version: "1.0.0", setup: () => {} },
      disposables: sink,
    });

    usePluginStore.getState().unload("alpha");

    expect(usePluginStore.getState().loaded.has("alpha")).toBe(false);
    expect(usePluginStore.getState().toolPreviews.size).toBe(0);
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
      host.tool.registerPreview("bash", StubPreview);

      usePluginStore.getState().registerLoaded({
        spec: { name: "alpha", version: "1.0.0", setup: () => {} },
        disposables: sink,
      });

      usePluginStore.getState().unload("alpha");

      // The good disposer still ran — "bash" is gone.
      expect(usePluginStore.getState().toolPreviews.size).toBe(0);
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
