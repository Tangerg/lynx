// Central Zustand store. React components subscribe so registrations
// propagate live. The state shape + map helpers live in registryState.ts /
// registryHelpers.ts; this file is the action implementations only.
//
// Every user-facing register* surface lives on the shared `extensions`
// substrate now (see kernelPoints.ts + host.contribute). What remains here:
// the open-extension-point add/remove, the declared-* placeholder maps (one
// `ownedSpecSlot` factory), and plugin bookkeeping (loaded / pendingActivations
// / window / appReady + the lifecycle-firing loops).

import type { Owned, PluginStoreActions, PluginStoreState } from "./registryState";
import type {
  ContributedCommand,
  ContributedSettingsPane,
  ContributedView,
  PluginSpec,
  ReadyHandler,
} from "./types";
import { create } from "zustand";
import { safeCall } from "./errors";
import { LIFECYCLE_POINT_IDS } from "./pointIds";
import {
  addOwned,
  clearByPlugin,
  mapDrop,
  mapSet,
  ownedContributionsTo,
  removeOwned,
} from "./registryHelpers";
import { freshState } from "./registryState";

type OwnedMapKey = {
  [K in keyof PluginStoreState]: PluginStoreState[K] extends Map<string, Owned<unknown>>
    ? K
    : never;
}[keyof PluginStoreState];

export const usePluginStore = create<PluginStoreState & PluginStoreActions>((set, get) => {
  // The only contribution maps still kept by name are the "declared
  // placeholder" surfaces (contributes.* awaiting activation). Every
  // register* surface lives on the shared `extensions` substrate; one factory
  // generates their id-keyed `{add,remove}`. The `as` casts narrow
  // PluginStoreState[K] from its TS union back to `Map<string, Owned<T>>`.
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

  const declaredCommands = ownedSpecSlot<ContributedCommand>(
    "declaredCommands",
    "declared command",
  );
  const declaredViews = ownedSpecSlot<ContributedView>("declaredViews", "declared view");
  const declaredSettingsPanes = ownedSpecSlot<ContributedSettingsPane>(
    "declaredSettingsPanes",
    "declared settings pane",
  );

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

    addDeclaredCommand: declaredCommands.add,
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
      syncDocumentTitle(text, get().windowBadge, get().windowWorking);
    },
    setWindowBadge(n) {
      set({ windowBadge: n });
      syncDocumentTitle(get().windowTitle, n, get().windowWorking);
    },
    setWindowWorking(on) {
      set({ windowWorking: on });
      syncDocumentTitle(get().windowTitle, get().windowBadge, on);
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
// badge + working state — the single composer of the document title, so a
// "working" indicator and a count badge can't clobber each other. Run only in
// DOM environments — test runs without a document just skip the assignment.
function syncDocumentTitle(base: string, badge: number, working: boolean): void {
  if (typeof document === "undefined") return;
  const dot = working ? "● " : "";
  const count = badge > 0 ? `(${badge}) ` : "";
  document.title = `${dot}${count}${base || "Lyra"}`;
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
