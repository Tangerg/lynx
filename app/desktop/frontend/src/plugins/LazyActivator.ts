// Placeholder component factory used when the kernel renders a
// surface (workspace view, settings pane, …) that belongs to a plugin
// whose `setup()` hasn't run yet.
//
// Mounting the placeholder fires `onActivate()` once; the plugin runs
// setup, registers the real component, and the registry-driven selector
// (useWorkspaceViews / useSettingsPanes) re-emits a list where the
// real component replaces this placeholder.

import type {ComponentType} from "react";
import {  createElement, useEffect } from "react";

export function makeLazyActivator(label: string, onActivate: () => void): ComponentType {
  return function LazyActivator() {
    useEffect(() => {
      onActivate();
    }, []);
    return createElement("div", { className: "lazy-activator" }, `Activating ${label}…`);
  };
}
