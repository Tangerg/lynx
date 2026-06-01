// Placeholder component factory used when the kernel renders a
// surface (workspace view, settings pane, …) that belongs to a plugin
// whose `setup()` hasn't run yet.
//
// Mounting the placeholder fires `onActivate()` once; the plugin runs
// setup and registers the real component, so the merged list from the
// registry-driven selector (useWorkspaceViews / useSettingsPanes) now
// carries the real component in this placeholder's place.

import type { ComponentType } from "react";
import { createElement, useEffect } from "react";

export function makeLazyActivator(label: string, onActivate: () => void): ComponentType {
  return function LazyActivator() {
    useEffect(() => {
      onActivate();
    }, []);
    return createElement("div", { className: "lazy-activator" }, `Activating ${label}…`);
  };
}
