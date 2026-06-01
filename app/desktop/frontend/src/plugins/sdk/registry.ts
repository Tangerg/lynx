// Central Zustand store of every plugin contribution — React
// components subscribe so registrations propagate live. The state shape
// + map helpers live in registryState.ts; this file is the action
// implementations only.
//
// Two local factories (ownedSpecSlot / multiSlot) generate the
// `set({ ... })` bodies for the remaining bookkeeping maps (declared
// placeholders + composite-key hooks). Each slot expands to ≤ 1 line per
// action. Every user-facing register* surface lives on the `extensions`
// substrate now (see kernelPoints.ts).

import type { Owned, PluginStoreActions, PluginStoreState } from "./registryState";
import type {
  ContributedCommand,
  ContributedSettingsPane,
  ContributedView,
  CoreEventHandler,
  CustomEventHandler,
  LayoutSlotSpec,
  PluginSpec,
  ReadyHandler,
} from "./types";
import { create } from "zustand";
import { safeCall } from "./errors";
import { LIFECYCLE_POINT_IDS } from "./pointIds";
import {
  addOwned,
  addOwnedMulti,
  clearByPlugin,
  mapDrop,
  mapSet,
  ownedContributionsTo,
  removeOwned,
  removeOwnedMulti,
} from "./registryHelpers";
import { freshState } from "./registryState";

type OwnedMapKey = {
  [K in keyof PluginStoreState]: PluginStoreState[K] extends Map<string, Owned<unknown>>
    ? K
    : never;
}[keyof PluginStoreState];

export const usePluginStore = create<PluginStoreState & PluginStoreActions>((set, get) => {
  // Slot factories. Each returns `{ add, remove }` closures that bake
  // in the state-key, label, and helper choice (`addOwned` vs
  // `addOwnedMulti`). The `as` casts narrow PluginStoreState[K] from
  // its TS union back to the concrete `Map<string, Owned<T>>`.

  // (pluginName, spec) — key is `spec.id`.
  function ownedSpecSlot<T>(slot: OwnedMapKey, label: string) {
    const keyOf = (s: T) => (s as unknown as { id: string }).id;
    return {
      add: (pluginName: string, spec: T) =>
        set({
          [slot]: addOwned(
            get()[slot] as Map<string, Owned<T>>,
            pluginName,
            keyOf(spec),
            spec,
            label,
          ),
        } as Partial<PluginStoreState>),
      remove: (pluginName: string, key: string) =>
        set({
          [slot]: removeOwned(get()[slot] as Map<string, Owned<T>>, pluginName, key),
        } as Partial<PluginStoreState>),
    };
  }

  // Composite-key multi-slot — same plugin may register many entries.
  function multiSlot<T>(slot: OwnedMapKey) {
    return {
      add: (pluginName: string, id: string, value: T) =>
        set({
          [slot]: addOwnedMulti(get()[slot] as Map<string, Owned<T>>, pluginName, id, value),
        } as Partial<PluginStoreState>),
      remove: (pluginName: string, id: string) =>
        set({
          [slot]: removeOwnedMulti(get()[slot] as Map<string, Owned<T>>, pluginName, id),
        } as Partial<PluginStoreState>),
    };
  }

  // Instantiate one helper per slot — kept dense; the action map below
  // wires them up to the public `addX` / `removeX` signatures. Only the
  // "declared placeholder" maps remain here — every register* contribution
  // surface now lives on the shared `extensions` substrate.
  const declaredCommands = ownedSpecSlot<ContributedCommand>(
    "declaredCommands",
    "declared command",
  );
  const declaredViews = ownedSpecSlot<ContributedView>("declaredViews", "declared view");
  const declaredSettingsPanes = ownedSpecSlot<ContributedSettingsPane>(
    "declaredSettingsPanes",
    "declared settings pane",
  );

  const customEvents = multiSlot<{ name: string; handler: CustomEventHandler<unknown> }>(
    "customEventHandlers",
  );
  const coreEvents = multiSlot<{ eventType: string; handler: CoreEventHandler }>(
    "coreEventHandlers",
  );
  const layoutSlots = multiSlot<{ slot: string; spec: LayoutSlotSpec }>("layoutSlots");

  return {
    ...freshState(),

    registerLoaded(plugin) {
      set({ loaded: mapSet(get().loaded, plugin.spec.name, plugin) });
      for (const o of ownedContributionsTo(get().extensions, LIFECYCLE_POINT_IDS.pluginLoad)) {
        const fn = o.value.item as (spec: PluginSpec) => void;
        safeCall(() => fn(plugin.spec), `[plugin] ${o.pluginName} onLoad listener threw:`);
      }
    },

    unload(pluginName) {
      const plugin = get().loaded.get(pluginName);
      if (!plugin) return;
      for (const d of plugin.disposables) {
        safeCall(() => d.dispose(), `[plugin] ${pluginName} dispose threw:`);
      }
      set({ loaded: mapDrop(get().loaded, pluginName) });
      for (const o of ownedContributionsTo(get().extensions, LIFECYCLE_POINT_IDS.pluginUnload)) {
        const fn = o.value.item as (name: string) => void;
        safeCall(() => fn(pluginName), `[plugin] ${o.pluginName} onUnload listener threw:`);
      }
    },

    addCustomEventHandler: (pluginName, name, id, handler) =>
      customEvents.add(pluginName, id, { name, handler }),
    removeCustomEventHandler: customEvents.remove,

    // eventType is baked into the composite key so the same plugin can
    // register handlers for several event types in one go.
    addCoreEventHandler: (pluginName, eventType, id, handler) =>
      coreEvents.add(pluginName, `${eventType}#${id}`, { eventType, handler }),
    removeCoreEventHandler: (pluginName, eventType, id) =>
      coreEvents.remove(pluginName, `${eventType}#${id}`),

    addLayoutSlot: (pluginName, slot, spec) =>
      layoutSlots.add(pluginName, `${slot}#${spec.id}`, { slot, spec }),
    removeLayoutSlot: (pluginName, slot, id) => layoutSlots.remove(pluginName, `${slot}#${id}`),

    addDeclaredCommand: declaredCommands.add,
    removeDeclaredCommand: declaredCommands.remove,
    removeDeclaredCommandsBy(pluginName) {
      set({ declaredCommands: clearByPlugin(get().declaredCommands, pluginName) });
    },

    addDeclaredView: declaredViews.add,
    removeDeclaredViewsBy(pluginName) {
      set({ declaredViews: clearByPlugin(get().declaredViews, pluginName) });
    },

    addDeclaredSettingsPane: declaredSettingsPanes.add,
    removeDeclaredSettingsPanesBy(pluginName) {
      set({ declaredSettingsPanes: clearByPlugin(get().declaredSettingsPanes, pluginName) });
    },

    addPendingActivation(spec, events) {
      set({ pendingActivations: mapSet(get().pendingActivations, spec.name, { spec, events }) });
    },
    removePendingActivation(name) {
      set({ pendingActivations: mapDrop(get().pendingActivations, name) });
    },

    markAppReady() {
      if (get().appReady) return;
      set({ appReady: true });
      for (const o of ownedContributionsTo(get().extensions, LIFECYCLE_POINT_IDS.ready)) {
        const fn = o.value.item as ReadyHandler;
        safeCall(() => fn(), `[plugin] ${o.pluginName} onReady threw:`);
      }
    },

    setWindowTitle(text) {
      set({ windowTitle: text });
      syncDocumentTitle(text, get().windowBadge);
    },
    setWindowBadge(n) {
      set({ windowBadge: n });
      syncDocumentTitle(get().windowTitle, n);
    },

    // Open extension points. The host computes `outerKey` from the point's
    // keying (single → `${point}#${dedupeKey}`, multi → `${point}#${plugin}|${id}`)
    // so the store stays oblivious to keying policy. `single` points warn on
    // cross-plugin override, mirroring `addOwned`.
    addContribution(pluginName, point, outerKey, entry, conflictKey) {
      const existing = get().extensions.get(outerKey);
      if (existing && existing.pluginName !== pluginName) {
        console.warn(
          `[plugin] ${pluginName} overrides contribution "${conflictKey}" ` +
            `on point "${point}" (previously ${existing.pluginName})`,
        );
      }
      set({ extensions: mapSet(get().extensions, outerKey, { pluginName, value: entry }) });
    },
    removeContribution(pluginName, outerKey) {
      const entry = get().extensions.get(outerKey);
      if (!entry || entry.pluginName !== pluginName) return;
      set({ extensions: mapDrop(get().extensions, outerKey) });
    },

    resetForTest() {
      set(freshState());
    },
  };
});

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
