// Central Zustand store of every plugin contribution — React
// components subscribe so registrations propagate live. The state shape
// + map helpers live in registryState.ts; this file is the action
// implementations only.

import { create } from "zustand";
import { safeCall } from "./errors";
import {
  addOwned,
  addOwnedMulti,
  clearByPlugin,
  freshState,
  removeOwned,
  removeOwnedMulti,
  type PluginStoreActions,
  type PluginStoreState,
} from "./registryState";

export const usePluginStore = create<PluginStoreState & PluginStoreActions>((set, get) => ({
  ...freshState(),

  registerLoaded(plugin) {
    const next = new Map(get().loaded);
    next.set(plugin.spec.name, plugin);
    set({ loaded: next });
    // Fan out to onLoad listeners — isolated per subscriber.
    for (const o of get().pluginLoadListeners.values()) {
      safeCall(
        () => o.value(plugin.spec),
        `[plugin] ${o.pluginName} onLoad listener threw:`,
      );
    }
  },

  unload(pluginName) {
    const plugin = get().loaded.get(pluginName);
    if (!plugin) return;
    for (const d of plugin.disposables) {
      safeCall(() => d.dispose(), `[plugin] ${pluginName} dispose threw:`);
    }
    const next = new Map(get().loaded);
    next.delete(pluginName);
    set({ loaded: next });
    for (const o of get().pluginUnloadListeners.values()) {
      safeCall(
        () => o.value(pluginName),
        `[plugin] ${o.pluginName} onUnload listener threw:`,
      );
    }
  },

  addToolPreview(pluginName, fn, component) {
    set({ toolPreviews: addOwned(get().toolPreviews, pluginName, fn, component, "tool preview") });
  },
  removeToolPreview(pluginName, fn) {
    set({ toolPreviews: removeOwned(get().toolPreviews, pluginName, fn) });
  },

  addToolAction(pluginName, spec) {
    set({ toolActions: addOwned(get().toolActions, pluginName, spec.id, spec, "tool action") });
  },
  removeToolAction(pluginName, id) {
    set({ toolActions: removeOwned(get().toolActions, pluginName, id) });
  },

  addToolIcon(pluginName, fn, icon) {
    set({ toolIcons: addOwned(get().toolIcons, pluginName, fn, icon, "tool icon") });
  },
  removeToolIcon(pluginName, fn) {
    set({ toolIcons: removeOwned(get().toolIcons, pluginName, fn) });
  },

  addContentBlock(pluginName, kind, renderer) {
    set({
      contentBlocks: addOwned(get().contentBlocks, pluginName, kind, renderer, "content block"),
    });
  },
  removeContentBlock(pluginName, kind) {
    set({ contentBlocks: removeOwned(get().contentBlocks, pluginName, kind) });
  },

  addCustomEventHandler(pluginName, name, id, handler) {
    set({
      customEventHandlers: addOwnedMulti(get().customEventHandlers, pluginName, id, {
        name,
        handler,
      }),
    });
  },
  removeCustomEventHandler(pluginName, id) {
    set({ customEventHandlers: removeOwnedMulti(get().customEventHandlers, pluginName, id) });
  },

  addSlashCommand(pluginName, cmd, spec) {
    set({ slashCommands: addOwned(get().slashCommands, pluginName, cmd, spec, "slash command") });
  },
  removeSlashCommand(pluginName, cmd) {
    set({ slashCommands: removeOwned(get().slashCommands, pluginName, cmd) });
  },

  addSettingsPane(pluginName, spec) {
    set({
      settingsPanes: addOwned(get().settingsPanes, pluginName, spec.id, spec, "settings pane"),
    });
  },
  removeSettingsPane(pluginName, id) {
    set({ settingsPanes: removeOwned(get().settingsPanes, pluginName, id) });
  },

  // Core handlers + layout slots both pre-baked their discriminator
  // (eventType / slot) into the composite key so the same plugin can
  // register more than one entry per discriminator. We still bake it in,
  // but via the shared helper now.
  addCoreEventHandler(pluginName, eventType, id, handler) {
    set({
      coreEventHandlers: addOwnedMulti(get().coreEventHandlers, pluginName, `${eventType}#${id}`, {
        eventType,
        handler,
      }),
    });
  },
  removeCoreEventHandler(pluginName, eventType, id) {
    set({
      coreEventHandlers: removeOwnedMulti(
        get().coreEventHandlers,
        pluginName,
        `${eventType}#${id}`,
      ),
    });
  },

  addLayoutSlot(pluginName, slot, spec) {
    set({
      layoutSlots: addOwnedMulti(get().layoutSlots, pluginName, `${slot}#${spec.id}`, {
        slot,
        spec,
      }),
    });
  },
  removeLayoutSlot(pluginName, slot, id) {
    set({ layoutSlots: removeOwnedMulti(get().layoutSlots, pluginName, `${slot}#${id}`) });
  },

  addTheme(pluginName, spec) {
    set({ themes: addOwned(get().themes, pluginName, spec.id, spec, "theme") });
  },
  removeTheme(pluginName, id) {
    set({ themes: removeOwned(get().themes, pluginName, id) });
  },

  addAccent(pluginName, spec) {
    set({ accents: addOwned(get().accents, pluginName, spec.id, spec, "accent") });
  },
  removeAccent(pluginName, id) {
    set({ accents: removeOwned(get().accents, pluginName, id) });
  },

  addRoute(pluginName, spec) {
    set({ routes: addOwned(get().routes, pluginName, spec.id, spec, "route") });
  },
  removeRoute(pluginName, id) {
    set({ routes: removeOwned(get().routes, pluginName, id) });
  },

  addShortcut(pluginName, spec) {
    // Normalize on the way in so registration and lookup match regardless of
    // case ("mod+k" vs "Mod+K"). We keep the original spec.key in the value
    // for display purposes.
    const key = normalizeCombo(spec.key);
    set({ shortcuts: addOwned(get().shortcuts, pluginName, key, spec, "shortcut") });
  },
  removeShortcut(pluginName, key) {
    set({ shortcuts: removeOwned(get().shortcuts, pluginName, normalizeCombo(key)) });
  },

  addComposerStatus(pluginName, spec) {
    set({
      composerStatus: addOwned(get().composerStatus, pluginName, spec.id, spec, "composer status"),
    });
  },
  removeComposerStatus(pluginName, id) {
    set({ composerStatus: removeOwned(get().composerStatus, pluginName, id) });
  },

  addComposerMode(pluginName, spec) {
    set({
      composerModes: addOwned(get().composerModes, pluginName, spec.id, spec, "composer mode"),
    });
  },
  removeComposerMode(pluginName, id) {
    set({ composerModes: removeOwned(get().composerModes, pluginName, id) });
  },

  addComposerPlaceholder(pluginName, spec) {
    set({
      composerPlaceholders: addOwned(
        get().composerPlaceholders,
        pluginName,
        spec.id,
        spec,
        "composer placeholder",
      ),
    });
  },
  removeComposerPlaceholder(pluginName, id) {
    set({ composerPlaceholders: removeOwned(get().composerPlaceholders, pluginName, id) });
  },

  addComposerAttachmentSource(pluginName, spec) {
    set({
      composerAttachmentSources: addOwned(
        get().composerAttachmentSources,
        pluginName,
        spec.id,
        spec,
        "composer attachment source",
      ),
    });
  },
  removeComposerAttachmentSource(pluginName, id) {
    set({
      composerAttachmentSources: removeOwned(get().composerAttachmentSources, pluginName, id),
    });
  },

  addComposerKeyBinding(pluginName, spec) {
    // Normalize the key on the way in so registrations and lookups match
    // regardless of case ("Enter" vs "enter", "Cmd+Enter" vs "mod+enter").
    const key = normalizeCombo(spec.key);
    set({
      composerKeyBindings: addOwned(
        get().composerKeyBindings,
        pluginName,
        key,
        spec,
        "composer key binding",
      ),
    });
  },
  removeComposerKeyBinding(pluginName, key) {
    set({
      composerKeyBindings: removeOwned(get().composerKeyBindings, pluginName, normalizeCombo(key)),
    });
  },

  addSidebarSection(pluginName, spec) {
    set({
      sidebarSections: addOwned(
        get().sidebarSections,
        pluginName,
        spec.id,
        spec,
        "sidebar section",
      ),
    });
  },
  removeSidebarSection(pluginName, id) {
    set({ sidebarSections: removeOwned(get().sidebarSections, pluginName, id) });
  },

  addAgentSource(pluginName, spec) {
    set({ agentSources: addOwned(get().agentSources, pluginName, spec.id, spec, "agent source") });
  },
  removeAgentSource(pluginName, id) {
    set({ agentSources: removeOwned(get().agentSources, pluginName, id) });
  },

  addCommand(pluginName, spec) {
    set({ commands: addOwned(get().commands, pluginName, spec.id, spec, "command") });
  },
  removeCommand(pluginName, id) {
    set({ commands: removeOwned(get().commands, pluginName, id) });
  },

  addDeclaredCommand(pluginName, spec) {
    set({
      declaredCommands: addOwned(
        get().declaredCommands,
        pluginName,
        spec.id,
        spec,
        "declared command",
      ),
    });
  },
  removeDeclaredCommand(pluginName, id) {
    set({ declaredCommands: removeOwned(get().declaredCommands, pluginName, id) });
  },
  removeDeclaredCommandsBy(pluginName) {
    set({ declaredCommands: clearByPlugin(get().declaredCommands, pluginName) });
  },

  addDeclaredView(pluginName, spec) {
    set({
      declaredViews: addOwned(get().declaredViews, pluginName, spec.id, spec, "declared view"),
    });
  },
  removeDeclaredViewsBy(pluginName) {
    set({ declaredViews: clearByPlugin(get().declaredViews, pluginName) });
  },

  addDeclaredSettingsPane(pluginName, spec) {
    set({
      declaredSettingsPanes: addOwned(
        get().declaredSettingsPanes,
        pluginName,
        spec.id,
        spec,
        "declared settings pane",
      ),
    });
  },
  removeDeclaredSettingsPanesBy(pluginName) {
    set({ declaredSettingsPanes: clearByPlugin(get().declaredSettingsPanes, pluginName) });
  },

  addPendingActivation(spec, events) {
    const next = new Map(get().pendingActivations);
    next.set(spec.name, { spec, events });
    set({ pendingActivations: next });
  },
  removePendingActivation(name) {
    const next = new Map(get().pendingActivations);
    next.delete(name);
    set({ pendingActivations: next });
  },

  addDataProvider(pluginName, spec) {
    set({
      dataProviders: addOwned(get().dataProviders, pluginName, spec.key, spec, "data provider"),
    });
  },
  removeDataProvider(pluginName, key) {
    set({ dataProviders: removeOwned(get().dataProviders, pluginName, key) });
  },

  addSidebarRailItem(pluginName, spec) {
    set({
      sidebarRailItems: addOwned(
        get().sidebarRailItems,
        pluginName,
        spec.id,
        spec,
        "sidebar rail item",
      ),
    });
  },
  removeSidebarRailItem(pluginName, id) {
    set({ sidebarRailItems: removeOwned(get().sidebarRailItems, pluginName, id) });
  },

  addMessageRole(pluginName, spec) {
    set({ messageRoles: addOwned(get().messageRoles, pluginName, spec.id, spec, "message role") });
  },
  removeMessageRole(pluginName, id) {
    set({ messageRoles: removeOwned(get().messageRoles, pluginName, id) });
  },

  // RPC hooks, log subscribers, lifecycle, plugin observers — every
  // composite-key slot below has the exact same add/remove shape now,
  // factored through `addOwnedMulti` / `removeOwnedMulti`.
  addRpcBeforeRequest(pluginName, id, hook) {
    set({ rpcBeforeRequest: addOwnedMulti(get().rpcBeforeRequest, pluginName, id, hook) });
  },
  removeRpcBeforeRequest(pluginName, id) {
    set({ rpcBeforeRequest: removeOwnedMulti(get().rpcBeforeRequest, pluginName, id) });
  },
  addRpcAfterResponse(pluginName, id, hook) {
    set({ rpcAfterResponse: addOwnedMulti(get().rpcAfterResponse, pluginName, id, hook) });
  },
  removeRpcAfterResponse(pluginName, id) {
    set({ rpcAfterResponse: removeOwnedMulti(get().rpcAfterResponse, pluginName, id) });
  },

  addLogSubscriber(pluginName, id, fn) {
    set({ logSubscribers: addOwnedMulti(get().logSubscribers, pluginName, id, fn) });
  },
  removeLogSubscriber(pluginName, id) {
    set({ logSubscribers: removeOwnedMulti(get().logSubscribers, pluginName, id) });
  },

  addReadyHandler(pluginName, id, fn) {
    set({ readyHandlers: addOwnedMulti(get().readyHandlers, pluginName, id, fn) });
  },
  removeReadyHandler(pluginName, id) {
    set({ readyHandlers: removeOwnedMulti(get().readyHandlers, pluginName, id) });
  },

  addBeforeUnloadHandler(pluginName, id, fn) {
    set({ beforeUnloadHandlers: addOwnedMulti(get().beforeUnloadHandlers, pluginName, id, fn) });
  },
  removeBeforeUnloadHandler(pluginName, id) {
    set({ beforeUnloadHandlers: removeOwnedMulti(get().beforeUnloadHandlers, pluginName, id) });
  },

  markAppReady() {
    if (get().appReady) return;
    set({ appReady: true });
    // Fire each handler — isolated; one throw must not skip the rest.
    for (const o of get().readyHandlers.values()) {
      safeCall(() => o.value(), `[plugin] ${o.pluginName} onReady threw:`);
    }
  },

  addPluginLoadListener(pluginName, id, fn) {
    set({ pluginLoadListeners: addOwnedMulti(get().pluginLoadListeners, pluginName, id, fn) });
  },
  removePluginLoadListener(pluginName, id) {
    set({ pluginLoadListeners: removeOwnedMulti(get().pluginLoadListeners, pluginName, id) });
  },
  addPluginUnloadListener(pluginName, id, fn) {
    set({ pluginUnloadListeners: addOwnedMulti(get().pluginUnloadListeners, pluginName, id, fn) });
  },
  removePluginUnloadListener(pluginName, id) {
    set({ pluginUnloadListeners: removeOwnedMulti(get().pluginUnloadListeners, pluginName, id) });
  },

  addPluginErrorFallback(pluginName, spec) {
    set({
      pluginErrorFallbacks: addOwned(
        get().pluginErrorFallbacks,
        pluginName,
        spec.id,
        spec,
        "plugin error fallback",
      ),
    });
  },
  removePluginErrorFallback(pluginName, id) {
    set({ pluginErrorFallbacks: removeOwned(get().pluginErrorFallbacks, pluginName, id) });
  },

  setWindowTitle(text) {
    set({ windowTitle: text });
    syncDocumentTitle(text, get().windowBadge);
  },
  setWindowBadge(n) {
    set({ windowBadge: n });
    syncDocumentTitle(get().windowTitle, n);
  },

  addWorkspaceView(pluginName, spec) {
    set({
      workspaceViews: addOwned(get().workspaceViews, pluginName, spec.id, spec, "workspace view"),
    });
  },
  removeWorkspaceView(pluginName, id) {
    set({ workspaceViews: removeOwned(get().workspaceViews, pluginName, id) });
  },

  resetForTest() {
    set(freshState());
  },
}));

// Side-effect: keep `document.title` in sync with the registry's title +
// badge. Run only in DOM environments — test runs without a document just
// skip the assignment.
function syncDocumentTitle(base: string, badge: number): void {
  if (typeof document === "undefined") return;
  const prefix = badge > 0 ? `(${badge}) ` : "";
  document.title = `${prefix}${base || "Lyra"}`;
}

// Modifier aliases → canonical name. `mod` collapses Cmd (Mac) / Ctrl
// (others) so registrations are cross-platform by default. Unmapped
// segments pass through unchanged (so a literal "ctrl+k" still records
// as ctrl, distinct from "mod+k").
const MODIFIER_ALIAS: Record<string, string> = {
  cmd: "mod",
  meta: "mod",
  mod: "mod",
  ctrl: "ctrl",
  control: "ctrl",
  shift: "shift",
  alt: "alt",
  option: "alt",
};

// Stable output order matches common docs convention (e.g. "mod+shift+k").
const MODIFIER_ORDER = ["mod", "ctrl", "alt", "shift"] as const;

// Normalize "Cmd+K" / "cmd+K" / "Mod+k" → canonical "mod+k" form. The
// leftmost segments are modifiers; the last segment is the key.
function normalizeCombo(combo: string): string {
  const parts = combo.split("+").map((p) => p.trim().toLowerCase());
  const key = parts.pop() ?? "";
  const mods = new Set<string>(parts.map((p) => MODIFIER_ALIAS[p] ?? p));
  const sortedMods = MODIFIER_ORDER.filter((m) => mods.has(m));
  return [...sortedMods, key].join("+");
}

export { normalizeCombo };
