// The registry's state shape + action signatures + the `freshState` factory.
// Pulled out of registry.ts so adding a new slot is a two-file edit (this
// file + registry.ts). Map mutation helpers live in `registryHelpers.ts`.

import type {
  ContributedCommand,
  ContributedSettingsPane,
  ContributedView,
  LoadedPlugin,
  PluginSpec,
} from "./types";

export interface Owned<T> {
  pluginName: string;
  value: T;
}

/**
 * One entry in the shared open-extension-point map. `point` is the point id
 * the entry belongs to; `key` is the dedupe key within that point (the
 * normalized single key, or the minted id for multi points) so consumers can
 * recover it without parsing the composite map key; `order` is the optional
 * sort hint passed at contribute time (the item may also carry its own
 * `order`); `item` is the contributed value. See `defineExtensionPoint` +
 * `selectors/extensions.ts`.
 */
export interface ContributionEntry {
  point: string;
  key: string;
  order?: number;
  item: unknown;
}

export interface PluginStoreState {
  loaded: Map<string, LoadedPlugin>;
  /** Commands declared in `PluginSpec.contributes.commands` but whose
   *  owning plugin hasn't been activated yet. Displayed as palette
   *  placeholders; running one activates the plugin first. */
  declaredCommands: Map<string, Owned<ContributedCommand>>;
  declaredViews: Map<string, Owned<ContributedView>>;
  declaredSettingsPanes: Map<string, Owned<ContributedSettingsPane>>;
  pendingActivations: Map<string, { spec: PluginSpec; events: string[] }>;
  appReady: boolean;
  // Open extension points — the unified substrate. Plugin-defined points
  // (and every kernel point) store their contributions here keyed by
  // `${point.id}#${dedupeKey}`. Read via the extensions selector.
  extensions: Map<string, Owned<ContributionEntry>>;
  windowTitle: string;
  windowBadge: number;
  windowWorking: boolean;
}

export interface PluginStoreActions {
  registerLoaded: (plugin: LoadedPlugin) => void;
  unload: (pluginName: string) => void;

  addDeclaredCommand: (pluginName: string, spec: ContributedCommand) => void;
  removeDeclaredCommandsBy: (pluginName: string) => void;

  addDeclaredView: (pluginName: string, spec: ContributedView) => void;
  removeDeclaredViewsBy: (pluginName: string) => void;

  addDeclaredSettingsPane: (pluginName: string, spec: ContributedSettingsPane) => void;
  removeDeclaredSettingsPanesBy: (pluginName: string) => void;

  addPendingActivation: (spec: PluginSpec, events: string[]) => void;
  removePendingActivation: (name: string) => void;

  /** Mark the app as ready — fires every contributed onReady hook in order. */
  markAppReady: () => void;

  // Open extension points (substrate). `outerKey` is the fully-qualified
  // `${point}#${dedupeKey}`; the host computes it (single vs multi keying)
  // and hands it back via the disposable so removal is exact.
  addContribution: (
    pluginName: string,
    point: string,
    outerKey: string,
    entry: ContributionEntry,
    conflictKey: string,
  ) => void;
  removeContribution: (pluginName: string, outerKey: string) => void;

  setWindowTitle: (text: string) => void;
  setWindowBadge: (n: number) => void;
  setWindowWorking: (on: boolean) => void;

  /**
   * Wipe the registry back to a fresh state. Only used by the test
   * harness — production code should never see this fire.
   */
  resetForTest: () => void;
}

export function freshState(): PluginStoreState {
  return {
    loaded: new Map(),
    declaredCommands: new Map(),
    declaredViews: new Map(),
    declaredSettingsPanes: new Map(),
    pendingActivations: new Map(),
    extensions: new Map(),
    appReady: false,
    windowTitle: "",
    windowBadge: 0,
    windowWorking: false,
  };
}
